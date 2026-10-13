package goslogx

import (
	"context"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TelemetryProvider supplies explicit providers to optional instrumentation.
// Implementations retain ownership of their providers.
type TelemetryProvider interface {
	TracerProvider() trace.TracerProvider
	MeterProvider() metric.MeterProvider
	TextMapPropagator() propagation.TextMapPropagator
}

var _ TelemetryProvider = (*Logger)(nil)

var disabledSpan trace.Span = noop.Span{}

func (l *Logger) Meter(name string, options ...metric.MeterOption) metric.Meter {
	if name == "" {
		name = instrumentationName
	}
	return l.MeterProvider().Meter(name, options...)
}

func Meter(name string, options ...metric.MeterOption) metric.Meter {
	return globalLog.Load().Meter(name, options...)
}

func (l *Logger) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	if name == "" {
		name = instrumentationName
	}
	if l == nil || l.configuredProvider == nil || (l.tracerProvider != nil && l.tracingClosed.Load()) {
		return noop.NewTracerProvider().Tracer(name, options...)
	}
	return l.configuredProvider.Tracer(name, options...)
}

func Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	return globalLog.Load().Tracer(name, options...)
}

func (l *Logger) StartSpan(ctx context.Context, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil || l.configuredProvider == nil || (l.tracerProvider != nil && l.tracingClosed.Load()) {
		return ctx, disabledSpan
	}
	return l.Tracer(instrumentationName).Start(ctx, name, options...)
}

func StartSpan(ctx context.Context, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	return globalLog.Load().StartSpan(ctx, name, options...)
}

// TextMapPropagator supplies W3C trace context to plugins without propagating
// potentially sensitive baggage. Legacy package helpers continue to support baggage.
func (l *Logger) TextMapPropagator() propagation.TextMapPropagator { return propagation.TraceContext{} }
