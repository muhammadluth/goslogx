package std

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestPublicEndpointCustomOptionsAndSafeErrors(t *testing.T) {
	for _, conditional := range []bool{true, false} {
		t.Run(fmt.Sprint(conditional), func(t *testing.T) {
			p := testkit.New(t)
			remote := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
			link := trace.Link{SpanContext: remote, Attributes: []attribute.KeyValue{attribute.String("app.phase", "retry"), attribute.String("db.statement", "SECRET")}}
			opts := []Option{WithPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})), WithServerName("api"), WithSpanStartOptions(trace.WithLinks(link), trace.WithNewRoot()), WithTraceAttributes(attribute.String("app.component", "web"))}
			if conditional {
				opts = append(opts, WithPublicEndpointFn(func(*http.Request) bool { return true }))
			} else {
				opts = append(opts, WithPublicEndpoint())
			}
			h := NewHandler(p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				span := trace.SpanFromContext(r.Context())
				if span.TracerProvider() == nil {
					t.Fatal("provider missing")
				}
				if baggage.FromContext(r.Context()).Member("region").Value() != "test" {
					t.Fatal("custom propagation")
				}
				span.RecordError(errors.New("SECRET"))
				span.AddLink(link)
				span.AddEvent("SECRET")
				span.AddEvent("read")
				w.WriteHeader(503)
			}), "GET /orders", opts...)
			req := httptest.NewRequest("GET", "/orders?token=SECRET", nil)
			req.Header.Set("traceparent", "00-01000000000000000000000000000000-0200000000000000-01")
			req.Header.Set("baggage", "region=test")
			h.ServeHTTP(httptest.NewRecorder(), req)
			spans := p.Recorder.Ended()
			if len(spans) != 1 || spans[0].Parent().IsValid() || len(spans[0].Links()) < 2 {
				t.Fatal("root/link relationships")
			}
			if attrs := fmt.Sprint(spans[0].Attributes()); !strings.Contains(attrs, "app.component") || !strings.Contains(attrs, "api") {
				t.Fatal("configured attributes", attrs)
			}
			if len(spans[0].Events()) != 2 || spans[0].Events()[0].Name != "exception" {
				t.Fatal("events lost or disabled read emitted", spans[0].Events())
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	if InstrumentClient(nil, nil) == nil {
		t.Fatal("nil client fallback")
	}
}

func TestDirectOtelHTTPSpanParity(t *testing.T) {
	p := testkit.New(t)
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
	defer parent.End()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	direct := otelhttp.NewHandler(next, "GET /orders", otelhttp.WithTracerProvider(p.TP), otelhttp.WithMeterProvider(p.MP), otelhttp.WithPropagators(p.TextMapPropagator()), otelhttp.WithSpanNameFormatter(func(operation string, _ *http.Request) string { return operation }))
	adapter := NewHandler(p, next, "GET /orders")
	for _, h := range []http.Handler{direct, adapter} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "http://localhost/orders", nil).WithContext(ctx))
	}
	a, b := p.Recorder.Ended()[0], p.Recorder.Ended()[1]
	if a.Name() != b.Name() || a.SpanKind() != b.SpanKind() || !a.Parent().Equal(b.Parent()) || a.Status() != b.Status() {
		t.Fatalf("HTTP span semantics changed: direct=%s/%s/%v/%v adapter=%s/%s/%v/%v", a.Name(), a.SpanKind(), a.Parent(), a.Status(), b.Name(), b.SpanKind(), b.Parent(), b.Status())
	}
}
