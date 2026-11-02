// Package opensearch integrates the official client's HTTP transport without a second tracing layer.
package opensearch

import (
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	goslogxhttp "github.com/muhammadluth/goslogx/plugins/http/std"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"net/http"
	"strings"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/database/opensearch"

type Option = telemetry.Option

var WithTraceAttributes = telemetry.WithTraceAttributes
var WithMetricAttributes = telemetry.WithMetricAttributes
var WithPropagator = telemetry.WithPropagator
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

// NewTransport fits the official client Config.Transport field. It preserves the
// base transport and uses HTTP spans/metrics, without inspecting query bodies.
func NewTransport(provider goslogx.TelemetryProvider, base http.RoundTripper, opts ...Option) http.RoundTripper {
	cfg := telemetry.Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	tp, mp, prop := telemetry.Resolve(provider, cfg)
	p := scopedProvider{tp: scopedTracer{tp}, mp: scopedMeter{mp}, prop: prop}
	return goslogxhttp.NewTransport(p, base, goslogxhttp.WithTraceAttributes(cfg.TraceAttributes...), goslogxhttp.WithMetricAttributes(cfg.MetricAttributes...), goslogxhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		operation := "request"
		switch {
		case strings.Contains(r.URL.Path, "/_search"):
			operation = "search"
		case strings.Contains(r.URL.Path, "/_bulk"):
			operation = "bulk"
		case strings.Contains(r.URL.Path, "/_update"):
			operation = "update"
		case r.Method == http.MethodDelete:
			operation = "delete"
		case strings.Contains(r.URL.Path, "/_doc"):
			operation = "index"
		}
		return "opensearch " + operation
	}))
}

type scopedProvider struct {
	tp   trace.TracerProvider
	mp   metric.MeterProvider
	prop propagation.TextMapPropagator
}

func (p scopedProvider) TracerProvider() trace.TracerProvider             { return p.tp }
func (p scopedProvider) MeterProvider() metric.MeterProvider              { return p.mp }
func (p scopedProvider) TextMapPropagator() propagation.TextMapPropagator { return p.prop }

type scopedTracer struct{ trace.TracerProvider }

func (p scopedTracer) Tracer(_ string, opts ...trace.TracerOption) trace.Tracer {
	return p.TracerProvider.Tracer(ScopeName, opts...)
}

type scopedMeter struct{ metric.MeterProvider }

func (p scopedMeter) Meter(_ string, opts ...metric.MeterOption) metric.Meter {
	return p.MeterProvider.Meter(ScopeName, opts...)
}
