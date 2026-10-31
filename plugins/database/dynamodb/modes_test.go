package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awm "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/smithy-go/middleware"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
)

func TestProviderSignalsAndNativeSDKMetadata(t *testing.T) {
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			cfg := aws.Config{}
			if err := Instrument(&cfg, p, WithTracing(signals[0]), WithMetrics(signals[1]), WithDBName("orders")); err != nil {
				t.Fatal(err)
			}
			stack := middleware.NewStack("GetItem", nil)
			for _, install := range cfg.APIOptions {
				if err := install(stack); err != nil {
					t.Fatal(err)
				}
			}
			ctx := awm.SetOperationName(awm.SetServiceID(context.Background(), "DynamoDB"), "GetItem")
			value := new(int)
			result, _, err := stack.Initialize.HandleMiddleware(ctx, value, middleware.HandlerFunc(func(_ context.Context, in any) (any, middleware.Metadata, error) {
				return in, middleware.Metadata{}, nil
			}))
			if err != nil || result != value {
				t.Fatal("native middleware result changed")
			}
			spans := p.Recorder.Ended()
			if (len(spans) == 1) != signals[0] || (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
				t.Fatal("signal modes")
			}
			if signals[0] {
				attrs := fmt.Sprint(spans[0].Attributes())
				for _, key := range []string{"rpc.system.name", "DynamoDB/GetItem", "db.namespace", "orders"} {
					if !strings.Contains(attrs, key) {
						t.Fatal("native SDK metadata lost", attrs)
					}
				}
			}
		})
	}
	if Instrument(nil, nil) == nil {
		t.Fatal("nil config")
	}
	cfg := aws.Config{}
	otelaws.AppendMiddlewares(&cfg.APIOptions)
	if err := Instrument(&cfg, nil); err != nil {
		t.Fatal(err)
	}
	stack := middleware.NewStack("GetItem", nil)
	for _, install := range cfg.APIOptions[:len(cfg.APIOptions)-1] {
		if err := install(stack); err != nil {
			t.Fatal(err)
		}
	}
	if err := cfg.APIOptions[len(cfg.APIOptions)-1](stack); err == nil {
		t.Fatal("existing native instrumentation accepted")
	}
}
