// Package testkit provides in-memory OTel fixtures for optional plugin tests.
package testkit

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type Provider struct {
	TP         *sdktrace.TracerProvider
	MP         *sdkmetric.MeterProvider
	Recorder   *tracetest.SpanRecorder
	Reader     *sdkmetric.ManualReader
	Propagator propagation.TextMapPropagator
}

func (p *Provider) TracerProvider() trace.TracerProvider {
	if p.TP == nil {
		return nil
	}
	return p.TP
}
func (p *Provider) MeterProvider() metric.MeterProvider {
	if p.MP == nil {
		return nil
	}
	return p.MP
}
func (p *Provider) TextMapPropagator() propagation.TextMapPropagator { return p.Propagator }
func New(t testing.TB) *Provider {
	t.Helper()
	p := &Provider{Recorder: tracetest.NewSpanRecorder(), Reader: sdkmetric.NewManualReader(), Propagator: propagation.TraceContext{}}
	p.TP = sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(p.Recorder))
	p.MP = sdkmetric.NewMeterProvider(sdkmetric.WithReader(p.Reader))
	t.Cleanup(func() { _ = p.TP.Shutdown(context.Background()); _ = p.MP.Shutdown(context.Background()) })
	return p
}
func (p *Provider) Collect(t testing.TB) metricdata.ResourceMetrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := p.Reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	return data
}
func (p *Provider) AssertPrivate(t testing.TB, forbidden ...string) {
	t.Helper()
	text := fmt.Sprint(p.Collect(t))
	for _, span := range p.Recorder.Ended() {
		text += fmt.Sprint(span.Name(), span.Attributes(), span.Status(), span.Events())
	}
	for _, secret := range forbidden {
		if strings.Contains(text, secret) {
			t.Fatalf("telemetry contains forbidden value %q", secret)
		}
	}
}
