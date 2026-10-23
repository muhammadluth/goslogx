package httptelemetry

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

func TestAttributePrivacyPolicies(t *testing.T) {
	attrs := []attribute.KeyValue{
		attribute.String("http.route", "/orders/{id}"),
		attribute.String("error.type", "database password"),
		attribute.String("http.request.method", "GET"),
		attribute.Int("http.response.status_code", 503),
		attribute.Int("http.request.body.size", 10),
		attribute.Int("http.response.body.size", 20),
		attribute.String("url.scheme", "https"),
		attribute.String("network.protocol.name", "http"),
		attribute.String("network.protocol.version", "2"),
		attribute.String("url.full", "https://secret.example"),
		attribute.String("http.route", strings.Repeat("x", 257)),
		attribute.Int("http.response.status_code", 99),
	}
	safe := safeAttributes(attrs)
	if got := fmt.Sprint(safe); strings.Contains(got, "secret") || !strings.Contains(got, "http_error") || len(safe) != 9 {
		t.Fatalf("unexpected safe attributes (%d): %s", len(safe), got)
	}
	metrics := safeMetricAttributes(attrs)
	if got := fmt.Sprint(metrics); strings.Contains(got, "body.size") || len(metrics) != 7 {
		t.Fatalf("metric dimensions retained unsafe body sizes (%d): %s", len(metrics), got)
	}
	configured := EncodeMetricAttributes([]attribute.KeyValue{
		attribute.String("app.channel", "mobile"), attribute.String("service.version", "1"), attribute.String("url.full", "secret"),
	})
	if got := decodeMetricAttributes(append(configured, attribute.String("goslogx.configured.app.channel", strings.Repeat("x", 129)))); len(got) != 2 || got[0].Key != "app.channel" {
		t.Fatalf("configured attribute round trip failed: %#v", got)
	}
}

func TestPrivateTelemetryStripsRequestData(t *testing.T) {
	fixture := testkit.New(t)
	p := Resolve(fixture, "privacy-test", NewConfig(
		WithServerName("orders"),
		WithTraceAttributes(attribute.String("app.component", "api")),
		WithMetricAttributes(attribute.String("app.channel", "mobile")),
	))
	ctx, span := p.TP.Tracer("caller").Start(context.Background(), "GET /orders", trace.WithAttributes(attribute.String("url.full", "SECRET")))
	if span.TracerProvider() == nil {
		t.Fatal("wrapped span did not expose a tracer provider")
	}
	span.SetAttributes(attribute.String("http.route", "/orders/{id}"), attribute.String("client.address", "SECRET"))
	span.AddEvent("SECRET")
	span.RecordError(fmt.Errorf("SECRET"))
	span.End()

	meter := p.MP.Meter("caller")
	h, err := meter.Int64Histogram("request.bytes")
	if err != nil {
		t.Fatal(err)
	}
	h.Record(ctx, 12, metric.WithAttributes(attribute.String("url.full", "SECRET"), attribute.Int("http.request.body.size", 12), attribute.String("http.route", "/orders/{id}")))
	counter, err := meter.Int64Counter("requests")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(ctx, 1, metric.WithAttributes(EncodeMetricAttributes([]attribute.KeyValue{attribute.String("app.tier", "edge")})...))
	updown, err := meter.Int64UpDownCounter("inflight")
	if err != nil {
		t.Fatal(err)
	}
	updown.Add(ctx, 1, metric.WithAttributes(attribute.String("url.full", "SECRET")))
	floatHistogram, err := meter.Float64Histogram("latency")
	if err != nil {
		t.Fatal(err)
	}
	floatHistogram.Record(ctx, .1, metric.WithAttributes(attribute.String("url.full", "SECRET")))

	fixture.AssertPrivate(t, "SECRET")
	output := fmt.Sprint(fixture.Collect(t))
	for _, want := range []string{"orders", "mobile", "edge", "/orders/{id}", "request.bytes", "latency", "inflight"} {
		if !strings.Contains(output, want) {
			t.Fatalf("telemetry missing %q: %s", want, output)
		}
	}
}
