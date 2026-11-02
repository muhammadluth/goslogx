package fasthttp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	native "github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestNativeOptionsSignalsAndParentRestoration(t *testing.T) {
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
			ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
			defer parent.End()
			member, _ := baggage.NewMember("region", "test")
			b, _ := baggage.New(member)
			ctx = baggage.ContextWithBaggage(ctx, b)
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			var req native.Request
			req.SetRequestURI("http://localhost/SECRET")
			req.Header.SetMethod("GET")
			prop.Inject(ctx, headerCarrier{&req.Header})
			if len((headerCarrier{&req.Header}).Keys()) != 2 {
				t.Fatal("carrier fields")
			}
			var request native.RequestCtx
			request.Init(&req, nil, nil)
			request.SetUserValue(contextKey{}, ctx)
			h, err := NewHandler(p, func(c *native.RequestCtx) {
				if baggage.FromContext(Context(c)).Member("region").Value() != "test" {
					t.Fatal("propagator ignored")
				}
				c.SetStatusCode(204)
			}, nil, WithServerName("api"), WithRoute(strings.Repeat("x", 257)), WithTraceAttributes(attribute.String("app.component", "api")), WithSpanStartOptions(trace.WithAttributes(attribute.String("app.operation", "lookup"))), WithTracing(signals[0]), WithMetrics(signals[1]), WithPropagator(prop))
			if err != nil {
				t.Fatal(err)
			}
			h(&request)
			if Context(&request) != ctx {
				t.Fatal("previous request context not restored")
			}
			spans := p.Recorder.Ended()
			if (len(spans) == 1) != signals[0] || (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
				t.Fatal("independent signals")
			}
			if signals[0] {
				if spans[0].Name() != "GET" || spans[0].Status().Code != codes.Error {
					t.Fatal("route/cancellation semantics")
				}
				attrs := fmt.Sprint(spans[0].Attributes())
				if !strings.Contains(attrs, "app.operation") || !strings.Contains(attrs, "server.address") {
					t.Fatal("configured span options", attrs)
				}
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	if _, err := NewHandler(nil, nil); err == nil {
		t.Fatal("nil handler")
	}
}
