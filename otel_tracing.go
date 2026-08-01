package goslogx

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/muhammadluth/goslogx"

func noopEndTrace() {}

func setupTracing(cfg *Config) (trace.Tracer, *sdktrace.TracerProvider, error) {
	if !cfg.Tracing.Enabled {
		return nil, nil, nil
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", cfg.ServiceName),
		)),
	}

	if cfg.Tracing.OTLP {
		exporterOptions := make([]otlptracegrpc.Option, 0, 1)
		if endpoint := cfg.Tracing.Endpoint; endpoint != "" {
			if !strings.Contains(endpoint, "://") {
				endpoint = "http://" + endpoint
			}
			exporterOptions = append(exporterOptions, otlptracegrpc.WithEndpointURL(endpoint))
		}

		exporter, err := otlptracegrpc.New(context.Background(), exporterOptions...)
		if err != nil {
			return nil, nil, err
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}

	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider.Tracer(instrumentationName), provider, nil
}

// StartTrace starts a span using the tracer configured by New and returns a
// derived context plus an end function. It exposes no OpenTelemetry types to
// the application. If tracing is disabled or failed to initialize, it returns
// the input context and a no-op end function.
func (l *Logger) StartTrace(ctx context.Context, name string) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil || l.tracer == nil {
		return ctx, noopEndTrace
	}

	ctx, span := l.tracer.Start(ctx, name)
	return ctx, func() {
		span.End()
	}
}

func (l *Logger) ensureTraceContext(ctx context.Context, name string) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if trace.SpanContextFromContext(ctx).IsValid() {
		return ctx, noopEndTrace
	}
	return l.StartTrace(ctx, name)
}

// StartTrace starts a span using the global logger configured by New. The
// application does not need to import OpenTelemetry packages.
func StartTrace(ctx context.Context, name string) (context.Context, func()) {
	return globalLog.Load().StartTrace(ctx, name)
}

// Shutdown flushes pending spans and stops the logger-owned TracerProvider.
// Call it during application shutdown when OTLP tracing is enabled.
func (l *Logger) Shutdown(ctx context.Context) error {
	if l == nil || l.tracerProvider == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return l.tracerProvider.Shutdown(ctx)
}

// Shutdown flushes pending spans and stops the global logger's TracerProvider.
func Shutdown(ctx context.Context) error {
	return globalLog.Load().Shutdown(ctx)
}

// TracingError returns an error encountered while initializing tracing. Logging
// remains available when tracing initialization fails.
func (l *Logger) TracingError() error {
	if l == nil {
		return nil
	}
	return l.tracingErr
}

// TracingError returns an error encountered while initializing tracing on the
// global logger.
func TracingError() error {
	return globalLog.Load().TracingError()
}

// ExtractHTTPContext extracts W3C Trace Context and baggage from HTTP headers
// using the propagator configured by goslogx.
func ExtractHTTPContext(ctx context.Context, header http.Header) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(header))
}

// InjectHTTPContext injects W3C Trace Context and baggage into HTTP headers.
func InjectHTTPContext(ctx context.Context, header http.Header) {
	if ctx == nil {
		ctx = context.Background()
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}

func recordSpanError(ctx context.Context, err error) {
	if ctx == nil || err == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
