package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awm "github.com/aws/aws-sdk-go-v2/aws/middleware"
	native "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go/middleware"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestDirectAWSMiddlewareParity(t *testing.T) {
	for _, failure := range []error{nil, errors.New("SECRET failure")} {
		var spans [2]sdktrace.ReadOnlySpan
		for index, adapter := range []bool{false, true} {
			p := testkit.New(t)
			cfg := aws.Config{}
			if adapter {
				if err := Instrument(&cfg, p, WithAttributeBuilder(nil), WithAttributeBuilder(func(_ context.Context, in middleware.InitializeInput, out middleware.InitializeOutput) []attribute.KeyValue {
					if in.Parameters == nil || out.Result == nil {
						t.Fatal("attribute builder lost SDK inputs")
					}
					return []attribute.KeyValue{attribute.String("app.component", "orders"), attribute.String("app.secret", "SECRET")}
				})); err != nil {
					t.Fatal(err)
				}
			} else {
				otelaws.AppendMiddlewares(&cfg.APIOptions, otelaws.WithTracerProvider(p.TP), otelaws.WithTextMapPropagator(p.Propagator))
			}
			stack := middleware.NewStack("GetItem", nil)
			for _, install := range cfg.APIOptions {
				if err := install(stack); err != nil {
					t.Fatal(err)
				}
			}
			sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
			ctx := trace.ContextWithSpanContext(context.Background(), sc)
			ctx = awm.SetOperationName(awm.SetServiceID(ctx, "DynamoDB"), "GetItem")
			input := &native.GetItemInput{TableName: aws.String("orders"), ConsistentRead: aws.Bool(true), ProjectionExpression: aws.String("SECRET"), Key: map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "SECRET"}}}
			_, _, err := stack.Initialize.HandleMiddleware(ctx, input, middleware.HandlerFunc(func(_ context.Context, in any) (any, middleware.Metadata, error) {
				if in != input {
					t.Fatal("SDK input replaced")
				}
				return &native.GetItemOutput{}, middleware.Metadata{}, failure
			}))
			if err != failure {
				t.Fatal("SDK error identity")
			}
			ended := p.Recorder.Ended()
			if len(ended) != 1 {
				t.Fatal("duplicate or missing span", ended)
			}
			spans[index] = ended[0]
			if adapter {
				p.AssertPrivate(t, "SECRET")
			}
		}
		want, got := spans[0], spans[1]
		if got.Name() != want.Name() || got.SpanKind() != want.SpanKind() || !got.Parent().Equal(want.Parent()) || got.Status().Code != want.Status().Code || len(got.Events()) != len(want.Events()) {
			t.Fatal("AWS span semantics")
		}
		actual := map[attribute.Key]attribute.Value{}
		for _, a := range got.Attributes() {
			actual[a.Key] = a.Value
		}
		for _, a := range want.Attributes() {
			switch a.Key {
			case "rpc.system.name", "rpc.method", "aws.dynamodb.table_names", "aws.dynamodb.consistent_read":
				if !reflect.DeepEqual(actual[a.Key], a.Value) {
					t.Fatal("native AWS metadata lost", a, got.Attributes())
				}
			}
		}
		if actual["app.component"].AsString() != "orders" {
			t.Fatal("custom attribute builder missing")
		}
		if actual["aws.dynamodb.table_names"].Type() != attribute.STRINGSLICE {
			t.Fatal("missing DynamoDB table metadata")
		}
	}
}

func TestSDKTransportPropagationAndConfiguredAttributes(t *testing.T) {
	p := testkit.New(t)
	requestHeaders := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHeaders <- r.Header.Clone()
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{"Item":{"id":{"S":"ok"}}}`))
	}))
	defer server.Close()
	cfg := aws.Config{Region: "ap-southeast-1", Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
	}), HTTPClient: server.Client()}
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	if err := Instrument(&cfg, p, WithPropagator(prop), WithTraceAttributes(attribute.String("app.component", "store")), WithMetricAttributes(attribute.String("app.region", "local"))); err != nil {
		t.Fatal(err)
	}
	client := native.NewFromConfig(cfg, func(options *native.Options) { options.BaseEndpoint = aws.String(server.URL) })
	member, _ := baggage.NewMember("region", "local")
	bag, _ := baggage.New(member)
	ctx := baggage.ContextWithBaggage(context.Background(), bag)
	result, err := client.GetItem(ctx, &native.GetItemInput{TableName: aws.String("orders"), Key: map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "SECRET"}}})
	if err != nil || result.Item["id"].(*types.AttributeValueMemberS).Value != "ok" {
		t.Fatal("native SDK response", result, err)
	}
	headers := <-requestHeaders
	if headers.Get("traceparent") == "" || headers.Get("baggage") != "region=local" {
		t.Fatal("configured propagation missing", headers)
	}
	spans := p.Recorder.Ended()
	if len(spans) != 1 {
		t.Fatal("duplicate SDK spans", spans)
	}
	if !strings.Contains(fmt.Sprint(spans[0].Attributes()), "store") || !strings.Contains(fmt.Sprint(p.Collect(t)), "local") {
		t.Fatal("configured attributes lost")
	}
	p.AssertPrivate(t, "SECRET")
}
