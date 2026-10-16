package telemetry

import (
	lifecycle "github.com/muhammadluth/goslogx/internal/telemetry/lifecycle"
	metrics "github.com/muhammadluth/goslogx/internal/telemetry/metrics"
	resourceimpl "github.com/muhammadluth/goslogx/internal/telemetry/resource"
	tracing "github.com/muhammadluth/goslogx/internal/telemetry/tracing"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Config struct {
	ServiceName     string
	Tracing         tracing.Config
	Metrics         metrics.Config
	Resource        resourceimpl.Config
	TracerProvider  trace.TracerProvider
	MeterProvider   metric.MeterProvider
	InternalMetrics bool
}
type Runtime struct {
	Tracer                 trace.Tracer
	ConfiguredProvider     trace.TracerProvider
	MeterProvider          metric.MeterProvider
	InternalMetrics        *metrics.Recorder
	TracingErr, MetricsErr error
	Lifecycle              *lifecycle.State
}

func New(cfg Config, logger *zap.Logger) *Runtime {
	r := &Runtime{MeterProvider: cfg.MeterProvider}
	identity := resourceimpl.Resolve(cfg.Resource, resourceimpl.Config{ServiceVersion: cfg.Tracing.ServiceVersion, DeploymentEnvironment: cfg.Tracing.DeploymentEnvironment, ServiceInstanceID: cfg.Tracing.ServiceInstanceID, ResourceAttributes: cfg.Tracing.ResourceAttributes})
	var serviceResource *resource.Resource
	var resourceErr error
	if cfg.Tracing.Enabled || cfg.Metrics.Enabled {
		serviceResource, resourceErr = resourceimpl.New(cfg.ServiceName, identity)
	}
	ownedMetrics, err := metrics.Setup(cfg.Metrics, cfg.MeterProvider, serviceResource, resourceErr)
	r.MetricsErr = err
	if err != nil {
		r.MeterProvider = nil
	} else if ownedMetrics != nil {
		r.MeterProvider = ownedMetrics
	}
	if r.MeterProvider == nil {
		r.MeterProvider = metricnoop.NewMeterProvider()
	}
	if cfg.InternalMetrics {
		r.InternalMetrics = metrics.NewRecorder(r.MeterProvider)
	}
	var ownedTracing *sdktrace.TracerProvider
	r.Tracer, ownedTracing, r.TracingErr = tracing.Setup(cfg.Tracing, cfg.TracerProvider, serviceResource, resourceErr)
	if r.TracingErr == nil && cfg.TracerProvider != nil {
		r.ConfiguredProvider = cfg.TracerProvider
	} else if ownedTracing != nil {
		r.ConfiguredProvider = ownedTracing
	}
	r.Lifecycle = lifecycle.New(ownedTracing, ownedMetrics, r.InternalMetrics)
	r.Lifecycle.Register(logger, cfg.Tracing.RegisterGlobal, cfg.Metrics.RegisterGlobal, cfg.Tracing.ErrorLogInterval, cfg.Metrics.ErrorLogInterval)
	return r
}
