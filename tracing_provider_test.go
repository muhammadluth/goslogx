package goslogx

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type ownershipSpanExporter struct {
	exported     atomic.Int64
	shutdownCall atomic.Int64
}

func (e *ownershipSpanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.exported.Add(int64(len(spans)))
	return nil
}

func (e *ownershipSpanExporter) Shutdown(context.Context) error {
	e.shutdownCall.Add(1)
	return nil
}

func TestExternalTracerProviderOwnership(t *testing.T) {
	exporter := &ownershipSpanExporter{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	globalBefore := otel.GetTracerProvider()
	logger := setupLog(
		WithOutput(&discardWriter{}),
		WithTracerProvider(provider),
	)
	if err := logger.TracingError(); err != nil {
		t.Fatalf("TracingError() = %v", err)
	}
	if logger.TracerProvider() != provider {
		t.Fatal("TracerProvider() did not return the external provider")
	}
	if otel.GetTracerProvider() != globalBefore {
		t.Fatal("WithTracerProvider replaced the global provider")
	}

	ctx, end := logger.StartTrace(context.Background(), "external-provider-span")
	if !sdktraceReadOnlySpanContext(ctx) {
		t.Fatal("StartTrace() did not use the external provider")
	}
	end()
	if got := exporter.exported.Load(); got != 1 {
		t.Fatalf("exported spans = %d, want 1", got)
	}

	if err := logger.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if got := exporter.shutdownCall.Load(); got != 0 {
		t.Fatalf("external provider shutdown calls = %d, want 0", got)
	}

	_, span := provider.Tracer(instrumentationName).Start(context.Background(), "after-logger-shutdown")
	span.End()
	if got := exporter.exported.Load(); got != 2 {
		t.Fatalf("exported spans after logger shutdown = %d, want 2", got)
	}
}

func TestTracerProviderConfigurationIsUnambiguous(t *testing.T) {
	external := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = external.Shutdown(context.Background()) })

	logger := setupLog(
		WithOutput(&discardWriter{}),
		WithTracerProvider(external),
		WithTracing(validManagedTracingConfig(false)),
	)
	err := logger.TracingError()
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("TracingError() = %v, want ambiguous provider error", err)
	}
	if logger.TracerProvider() != nil {
		t.Fatal("ambiguous configuration exposed a provider")
	}
}

func TestManagedProviderGlobalRegistrationIsExplicit(t *testing.T) {
	previous := otel.GetTracerProvider()
	marker := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(marker)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = marker.Shutdown(context.Background())
	})

	t.Run("default leaves global provider unchanged", func(t *testing.T) {
		logger := setupLog(
			WithOutput(&discardWriter{}),
			WithServiceName("payment-api"),
			WithTracing(validManagedTracingConfig(false)),
		)
		if err := logger.TracingError(); err != nil {
			t.Fatalf("TracingError() = %v", err)
		}
		if otel.GetTracerProvider() != marker {
			t.Fatal("managed provider replaced global provider without RegisterGlobal")
		}
		if err := logger.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown() error = %v", err)
		}
	})

	t.Run("explicit registration is restored on shutdown", func(t *testing.T) {
		logger := setupLog(
			WithOutput(&discardWriter{}),
			WithServiceName("payment-api"),
			WithTracing(validManagedTracingConfig(true)),
		)
		if err := logger.TracingError(); err != nil {
			t.Fatalf("TracingError() = %v", err)
		}
		if otel.GetTracerProvider() != logger.TracerProvider() {
			t.Fatal("RegisterGlobal did not install the managed provider")
		}
		if err := logger.Shutdown(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Shutdown() error = %v", err)
		}
		if otel.GetTracerProvider() != marker {
			t.Fatal("Shutdown() did not restore the previous global provider")
		}
	})
}

func validManagedTracingConfig(registerGlobal bool) TracingConfig {
	return TracingConfig{
		Endpoint:              "127.0.0.1:4317",
		Insecure:              true,
		RegisterGlobal:        registerGlobal,
		ServiceVersion:        "1.0.0",
		DeploymentEnvironment: "test",
		ServiceInstanceID:     "test-instance",
	}
}
