package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type providers struct {
	tp trace.TracerProvider
	mp metric.MeterProvider
}

func (p providers) TracerProvider() trace.TracerProvider { return p.tp }
func (p providers) MeterProvider() metric.MeterProvider  { return p.mp }
func (p providers) TextMapPropagator() propagation.TextMapPropagator {
	return propagation.TraceContext{}
}
func TestPropagationPrivacyAndCardinality(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	p := providers{tp, mp}
	producer, err := New(p, "producer", "nats", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, finish := producer.Begin(context.Background(), "publish", "orders.SECRET", trace.SpanKindProducer)
	headers := propagation.MapCarrier{}
	producer.Propagator.Inject(ctx, headers)
	finish(nil)
	finish(errors.New("SECRET"))
	consumer, err := New(p, "consumer", "nats", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.Process(context.Background(), headers, "orders.SECRET", func(ctx context.Context) error { return errors.New("SECRET") }); err == nil {
		t.Fatal("handler error lost")
	}
	spans := recorder.Ended()
	if len(spans) != 2 || spans[1].Name() != "nats process" || spans[1].Parent().SpanID() != spans[0].SpanContext().SpanID() {
		t.Fatalf("chain: %v", spans)
	}
	for _, s := range spans {
		if strings.Contains(fmt.Sprint(s.Name(), s.Attributes(), s.Status(), s.Events()), "SECRET") {
			t.Fatal("span leak")
		}
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprint(data), "SECRET") {
		t.Fatal("metric leak")
	}
	bounded, _ := New(p, "bounded", "nats", true, WithDestinationNormalizer(func(s string) string { return s }))
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); bounded.Destination(fmt.Sprint(i)) }(i)
	}
	wg.Wait()
	if len(bounded.destinations) != 64 || bounded.Destination("overflow") != "_OTHER" {
		t.Fatal("unbounded dimensions")
	}
}
func TestDisabledAndStatementOptIn(t *testing.T) {
	if _, err := New(nil, "test", "postgresql", false, WithStatementCapture(true)); err == nil {
		t.Fatal("capture without sanitizer accepted")
	}
	t1, err := New(nil, "test", "postgresql", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := t1.Run(context.Background(), "SELECT", trace.SpanKindClient, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseDiagnosticMetadataStaysOnSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	inst, err := New(providers{tp, mp}, "database", "clickhouse", false,
		WithDBName("orders"), WithDBUser("diagnostic-user"),
		WithServerEndpoint("database.internal", 9000), WithCallerEnabled(true))
	if err != nil {
		t.Fatal(err)
	}
	_, end := inst.Begin(context.Background(), "SELECT", "", trace.SpanKindClient)
	end(nil)
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatal("database client span missing")
	}
	attrs := fmt.Sprint(spans[0].Attributes())
	for _, wanted := range []string{"db.namespace", "db.name", "diagnostic-user", "database.internal", "server.port", "code.function"} {
		if !strings.Contains(attrs, wanted) {
			t.Fatalf("missing %s: %s", wanted, attrs)
		}
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	metricText := fmt.Sprint(data)
	for _, forbidden := range []string{"diagnostic-user", "database.internal", "code.filepath", "db.name {"} {
		if strings.Contains(metricText, forbidden) {
			t.Fatalf("trace metadata in metric dimensions: %s", metricText)
		}
	}
}

func TestIndependentSignalsAndSafeStatements(t *testing.T) {
	for _, tc := range []struct {
		name            string
		traces, metrics bool
	}{{"both", true, true}, {"traces", true, false}, {"metrics", false, true}, {"neither", false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			defer tp.Shutdown(context.Background())
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer mp.Shutdown(context.Background())
			inst, err := New(providers{tp, mp}, "signal-test", "postgresql", false, WithTracing(tc.traces), WithMetrics(tc.metrics), WithStatementCapture(true), WithStatementSanitizer(func(string) string { return "SELECT ?" }))
			if err != nil {
				t.Fatal(err)
			}
			sentinel := errors.New("SECRET")
			if err = inst.Run(context.Background(), "SELECT", trace.SpanKindClient, func(ctx context.Context) error { inst.Statement(ctx, "SELECT 'SECRET'"); return sentinel }); err != sentinel {
				t.Fatal("error identity")
			}
			if (len(recorder.Ended()) > 0) != tc.traces {
				t.Fatal("trace toggle")
			}
			var data metricdata.ResourceMetrics
			if err = reader.Collect(context.Background(), &data); err != nil {
				t.Fatal(err)
			}
			if (len(data.ScopeMetrics) > 0) != tc.metrics {
				t.Fatal("metric toggle")
			}
			exported := fmt.Sprint(data)
			for _, s := range recorder.Ended() {
				exported += fmt.Sprint(s.Attributes(), s.Status(), s.Events())
			}
			if strings.Contains(exported, "SECRET") {
				t.Fatal("secret exposed")
			}
		})
	}
}
