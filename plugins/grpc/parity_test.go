package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/stats"
)

func TestNativeClientStatsParityWithoutMethodAllowlist(t *testing.T) {
	p := testkit.New(t)
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
	defer parent.End()
	direct := otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(p.TP), otelgrpc.WithMeterProvider(p.MP), otelgrpc.WithMessageEvents(otelgrpc.SentEvents))
	adapter := NewClientHandler(p, WithMessageEvents(SentEvent))
	for _, handler := range []stats.Handler{direct, adapter} {
		child := handler.TagRPC(ctx, &stats.RPCTagInfo{FullMethodName: "/orders.v1.Orders/Create"})
		handler.HandleRPC(child, &stats.Begin{Client: true, BeginTime: time.Now()})
		handler.HandleRPC(child, &stats.OutPayload{Client: true, Length: 6, CompressedLength: 4, SentTime: time.Now()})
		handler.HandleRPC(child, &stats.End{Client: true, EndTime: time.Now()})
	}
	spans := p.Recorder.Ended()
	if len(spans) != 2 {
		t.Fatal("stats lifecycle", len(spans))
	}
	a, b := spans[0], spans[1]
	if a.Name() != b.Name() || a.SpanKind() != b.SpanKind() || !a.Parent().Equal(b.Parent()) || len(a.Events()) != len(b.Events()) {
		t.Fatal("native RPC parity")
	}
	attributes := map[attribute.Key]attribute.Value{}
	for _, attr := range b.Attributes() {
		attributes[attr.Key] = attr.Value
	}
	for _, attr := range a.Attributes() {
		if attr.Key == "rpc.service" || attr.Key == "rpc.method" || attr.Key == "rpc.system" || attr.Key == "rpc.grpc.status_code" {
			if attributes[attr.Key] != attr.Value {
				t.Fatal("native RPC attribute lost", attr)
			}
		}
	}
}

func TestPublicEndpointLinksWithRealRPC(t *testing.T) {
	p := testkit.New(t)
	conn := connect(t, p, &healthService{}, nil, WithPublicEndpointFn(func(context.Context, *stats.RPCTagInfo) bool { return true }), WithSpanAttributes(attribute.String("app.component", "health")))
	if _, err := grpc_health_v1.NewHealthClient(conn).Check(context.Background(), &grpc_health_v1.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for len(p.Recorder.Ended()) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	spans := p.Recorder.Ended()
	if len(spans) != 2 {
		t.Fatal("RPC completion")
	}
	for _, span := range spans {
		if span.SpanKind() == trace.SpanKindServer {
			if span.Parent().IsValid() || len(span.Links()) != 1 || !span.Links()[0].SpanContext.IsRemote() {
				t.Fatal("public endpoint relationship")
			}
		}
	}
}
