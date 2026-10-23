package httptelemetry

import (
	"context"
	"errors"
	"fmt"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"net/http"
	"strings"
	"testing"
)

func TestConfigOptionsAndNaming(t *testing.T) {
	traceAttrs := []attribute.KeyValue{attribute.String("app.component", "api")}
	metricAttrs := []attribute.KeyValue{
		attribute.String("app.channel", "web"),
		attribute.String("app.user", "untrusted"),
		attribute.Int("app.region", 1),
	}
	events := []MessageEvent{ReadEvent, WriteEvent}
	c := NewConfig(
		WithServerName("orders"), WithFilter(func(*http.Request) bool { return true }),
		WithPublicEndpointFn(func(*http.Request) bool { return true }),
		WithMetricAttributesFn(func(*http.Request) []attribute.KeyValue { return metricAttrs }),
		WithMessageEvents(events...), WithSpanNameFormatter(func(route string, _ *http.Request) string { return "custom " + route }),
		WithTraceAttributes(traceAttrs...), WithMetricAttributes(metricAttrs...),
		WithSpanStartOptions(trace.WithNewRoot()), WithPropagator(propagation.Baggage{}),
		WithTracing(false), WithMetrics(false), WithPublicEndpoint(), nil,
	)
	traceAttrs[0] = attribute.String("app.component", "changed")
	metricAttrs[0] = attribute.String("app.channel", "changed")
	events[0] = WriteEvent
	if c.ServerName != "orders" || c.Filter == nil || c.PublicEndpointFn == nil || c.MetricAttributesFn == nil || !c.TracingDisabled || !c.MetricsDisabled || !c.Public {
		t.Fatalf("configuration options were not retained: %#v", c)
	}
	if got := c.TraceAttributes[0].Value.AsString(); got != "api" {
		t.Fatalf("trace attributes must be copied, got %q", got)
	}
	if len(c.MetricAttributes) != 1 || c.MetricAttributes[0].Key != "app.channel" || len(c.MessageEvents) != 2 || c.MessageEvents[0] != ReadEvent || c.Propagator == nil {
		t.Fatalf("configuration privacy or copies failed: %#v", c)
	}
	if got := NewConfig(WithServerName(strings.Repeat("x", 257))).ServerName; got != "" {
		t.Fatalf("oversized server name accepted: %q", got)
	}
	for input, want := range map[string]string{"GET": "GET", "BREW": "_OTHER", "": "_OTHER"} {
		if got := Method(input); got != want {
			t.Errorf("Method(%q) = %q, want %q", input, got, want)
		}
	}
	if got := Name("GET", "/orders"); got != "GET /orders" {
		t.Fatalf("route name = %q", got)
	}
	if got := Name("BREW", strings.Repeat("/", 257)); got != "_OTHER" {
		t.Fatalf("unsafe route name = %q", got)
	}
}

func TestNilAndProviderAccessors(t *testing.T) {
	var pointer *int
	if !Nil(pointer) || !Nil((func())(nil)) || Nil(0) {
		t.Fatal("Nil did not classify values correctly")
	}
	p := Provider{TP: tracenoop.NewTracerProvider(), MP: metricnoop.NewMeterProvider(), Prop: propagation.Baggage{}}
	if p.TracerProvider() != p.TP || p.MeterProvider() != p.MP || p.TextMapPropagator() != p.Prop {
		t.Fatal("provider accessors changed provider contracts")
	}
}

func TestServerMetricsRecordsSafeDimensions(t *testing.T) {
	fixture := testkit.New(t)
	p := Resolve(fixture, "metrics-test", NewConfig(WithMetricAttributes(attribute.String("app.channel", "mobile"))))
	m, err := NewServerMetrics(p, NewConfig(WithMetricAttributes(attribute.String("app.channel", "mobile"))))
	if err != nil {
		t.Fatal(err)
	}
	finish := m.Begin(context.Background(), "BREW")
	finish("/orders/{id}", 503, false)
	finish = m.Begin(context.Background(), "GET")
	finish(strings.Repeat("/", 257), 99, true)
	data := fmt.Sprint(fixture.Collect(t))
	for _, want := range []string{"http.server.request.duration", "http.server.active_requests", "_OTHER", "/orders/{id}", "503", "http_error", "mobile"} {
		if !strings.Contains(data, want) {
			t.Fatalf("metrics missing %q: %s", want, data)
		}
	}
}

func TestSignalResolutionAndPrivacy(t *testing.T) {
	for _, tc := range []struct{ traces, metrics bool }{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			fixture := testkit.New(t)
			c := NewConfig(WithTracing(tc.traces), WithMetrics(tc.metrics), WithServerName("orders-api"), WithMetricAttributes(attribute.String("app.channel", "mobile"), attribute.String("app.user_id", "SECRET")), WithTraceAttributes(attribute.String("app.component", "handler")))
			p := Resolve(fixture, "test-http", c)
			ctx, span := p.TP.Tracer("upstream").Start(context.Background(), "GET /orders/{id}", trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String("url.full", "SECRET"), attribute.String("http.route", "/orders/{id}")))
			span.RecordError(errors.New("SECRET"))
			span.AddEvent("SECRET")
			span.SetStatus(codes.Error, "SECRET")
			span.SetAttributes(attribute.String("client.address", "SECRET"))
			span.End()
			m := p.MP.Meter("upstream")
			h, err := m.Float64Histogram("http.server.request.duration")
			if err != nil {
				t.Fatal(err)
			}
			h.Record(ctx, .1, metric.WithAttributes(attribute.String("url.full", "SECRET"), attribute.String("http.route", "/orders/{id}")))
			a, err := m.Int64UpDownCounter("http.server.active_requests")
			if err != nil {
				t.Fatal(err)
			}
			a.Add(ctx, 1, metric.WithAttributes(attribute.String("url.full", "SECRET")))
			a.Add(ctx, -1)
			if (len(fixture.Recorder.Ended()) > 0) != tc.traces {
				t.Fatal("tracing toggle")
			}
			data := fixture.Collect(t)
			if (len(data.ScopeMetrics) > 0) != tc.metrics {
				t.Fatal("metrics toggle")
			}
			fixture.AssertPrivate(t, "SECRET")
			if tc.metrics && !strings.Contains(fmt.Sprint(data), "mobile") {
				t.Fatal("custom metric attribute")
			}
		})
	}
	var typedNil *testkit.Provider
	p := Resolve(typedNil, "nil", Config{})
	_, s := p.TP.Tracer("nil").Start(context.Background(), "nil")
	s.End()
}
