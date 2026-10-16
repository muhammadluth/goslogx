package goslogx

import (
	telemetryimpl "github.com/muhammadluth/goslogx/internal/telemetry"
	metricsimpl "github.com/muhammadluth/goslogx/internal/telemetry/metrics"
	resourceimpl "github.com/muhammadluth/goslogx/internal/telemetry/resource"
	tracingimpl "github.com/muhammadluth/goslogx/internal/telemetry/tracing"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// TracingProvider supplies tracing and generic context propagation.
type TracingProvider interface {
	TracerProvider() trace.TracerProvider
	TextMapPropagator() propagation.TextMapPropagator
}

// MetricsProvider supplies metrics without transferring provider ownership.
type MetricsProvider interface{ MeterProvider() metric.MeterProvider }

// TelemetryProvider is the public capability contract consumed by plugins.
type TelemetryProvider interface {
	TracingProvider
	MetricsProvider
}

var _ TelemetryProvider = (*Logger)(nil)

func newTelemetry(cfg *Config, logger *zap.Logger) *telemetryimpl.Runtime {
	return telemetryimpl.New(telemetryimpl.Config{ServiceName: cfg.ServiceName, Resource: resourceimpl.Config(cfg.Resource), Tracing: tracingimpl.Config(cfg.Tracing), Metrics: metricsimpl.Config(cfg.Metrics), TracerProvider: cfg.TracerProvider, MeterProvider: cfg.MeterProvider, InternalMetrics: cfg.InternalMetrics}, logger)
}
