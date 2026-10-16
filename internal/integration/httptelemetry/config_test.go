package httptelemetry

import (
	"context"
	"errors"
	"fmt"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"strings"
	"testing"
)

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
