package goslogx

import (
	"context"
	"crypto/tls"
	"time"

	lifecycleimpl "github.com/muhammadluth/goslogx/internal/telemetry/lifecycle"
	tracingimpl "github.com/muhammadluth/goslogx/internal/telemetry/tracing"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	instrumentationName         = tracingimpl.ScopeName
	DefaultSamplingRatio        = tracingimpl.DefaultSamplingRatio
	DefaultBatchTimeout         = tracingimpl.DefaultBatchTimeout
	DefaultExportTimeout        = tracingimpl.DefaultExportTimeout
	DefaultMaxQueueSize         = tracingimpl.DefaultMaxQueueSize
	DefaultMaxExportBatchSize   = tracingimpl.DefaultMaxExportBatchSize
	DefaultOTelErrorLogInterval = tracingimpl.DefaultOTelErrorLogInterval
)

var ErrTracingShutdown = lifecycleimpl.ErrTracingShutdown

func noopEndTrace() {}

// StartTrace starts a span using the tracer configured by New and returns a
// derived context plus an end function. It exposes no OpenTelemetry types to
// the application. If tracing is disabled or failed to initialize, it returns
// the input context and a no-op end function.
func (l *Logger) StartTrace(ctx context.Context, name string) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil || l.telemetry == nil || l.telemetry.Tracer == nil || (l.telemetry.Lifecycle.TracerProvider != nil && l.telemetry.Lifecycle.Closed.Load()) {
		return ctx, noopEndTrace
	}

	ctx, span := l.telemetry.Tracer.Start(ctx, name)
	return ctx, func() {
		span.End()
	}
}

// StartTrace starts a span using the global logger configured by New. The
// application does not need to import OpenTelemetry packages.
func StartTrace(ctx context.Context, name string) (context.Context, func()) {
	return globalLog.Load().StartTrace(ctx, name)
}

// TracerProvider returns the provider configured for this logger. The caller
// can pass it to instrumentation such as otelhttp or otelgrpc without changing
// OpenTelemetry global state. The returned provider may be application-owned.
func (l *Logger) TracerProvider() trace.TracerProvider {
	if l == nil || l.telemetry == nil {
		return nil
	}
	return l.telemetry.ConfiguredProvider
}

// TracerProvider returns the provider configured on the global logger.
func TracerProvider() trace.TracerProvider {
	return globalLog.Load().TracerProvider()
}

// TracingError returns an error encountered while initializing tracing. Logging
// remains available when tracing initialization fails.
func (l *Logger) TracingError() error {
	if l == nil || l.telemetry == nil {
		return nil
	}
	return l.telemetry.TracingErr
}

// TracingError returns an error encountered while initializing tracing on the
// global logger.
func TracingError() error {
	return globalLog.Load().TracingError()
}

var disabledSpan trace.Span = noop.Span{}

func (l *Logger) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	if name == "" {
		name = instrumentationName
	}
	if l == nil || l.telemetry == nil || l.telemetry.ConfiguredProvider == nil || (l.telemetry.Lifecycle.TracerProvider != nil && l.telemetry.Lifecycle.Closed.Load()) {
		return noop.NewTracerProvider().Tracer(name, options...)
	}
	return l.telemetry.ConfiguredProvider.Tracer(name, options...)
}
func Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	return globalLog.Load().Tracer(name, options...)
}
func (l *Logger) StartSpan(ctx context.Context, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil || l.telemetry == nil || l.telemetry.ConfiguredProvider == nil || (l.telemetry.Lifecycle.TracerProvider != nil && l.telemetry.Lifecycle.Closed.Load()) {
		return ctx, disabledSpan
	}
	return l.Tracer(instrumentationName).Start(ctx, name, options...)
}
func StartSpan(ctx context.Context, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	return globalLog.Load().StartSpan(ctx, name, options...)
}

// TracingConfig controls OpenTelemetry tracing initialization.
type TracingConfig struct {
	// Enabled creates an OpenTelemetry TracerProvider inside goslogx.
	Enabled bool

	// RegisterGlobal explicitly installs the goslogx-owned provider as the
	// OpenTelemetry global provider. The previous global provider is restored
	// during Shutdown. The default is false.
	RegisterGlobal bool

	// Endpoint is the OTLP/gRPC collector URL.
	Endpoint string

	// SamplingRatio is the fraction of root traces to sample, from 0.0 to 1.0.
	// The zero value uses DefaultSamplingRatio. Parent sampling decisions are
	// always honored.
	SamplingRatio float64

	// Insecure disables transport security. Use it only for trusted local
	// Collector connections.
	Insecure bool

	// TLSConfig configures TLS, including custom root CAs and mTLS certificates.
	// It cannot be combined with Insecure.
	TLSConfig *tls.Config

	// Headers are sent with every OTLP/gRPC export request. Values commonly
	// contain credentials and are never included in validation errors.
	Headers map[string]string

	// Compression optionally enables payload compression. The supported value
	// is "gzip"; an empty value disables compression.
	Compression string

	// ExportTimeout limits each OTLP export request. The zero value uses the
	// DefaultExportTimeout. It also bounds batch processor export calls.
	ExportTimeout time.Duration

	// BatchTimeout is the maximum delay before queued spans are exported. The
	// zero value uses DefaultBatchTimeout.
	BatchTimeout time.Duration

	// MaxQueueSize is the maximum number of spans waiting for export. New spans
	// are dropped when this non-blocking queue is full. The zero value uses
	// DefaultMaxQueueSize.
	MaxQueueSize int

	// MaxExportBatchSize is the maximum number of spans per export request. The
	// zero value uses DefaultMaxExportBatchSize, capped by MaxQueueSize.
	MaxExportBatchSize int

	// ServiceVersion identifies the deployed service build.
	ServiceVersion string

	// DeploymentEnvironment identifies the deployment environment, such as
	// production or staging.
	DeploymentEnvironment string

	// ServiceInstanceID uniquely identifies this service instance.
	ServiceInstanceID string

	// ResourceAttributes adds deployment-specific resource attributes. Reserved
	// service attributes cannot be overridden through this map.
	ResourceAttributes map[string]string

	// ErrorLogInterval limits repeated OpenTelemetry internal error logs. The
	// zero value uses DefaultOTelErrorLogInterval.
	ErrorLogInterval time.Duration
}
