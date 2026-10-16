package goslogx

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tracingimpl "github.com/muhammadluth/goslogx/internal/telemetry/tracing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.Write(p)
}

func (b *lockedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.Len()
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.String()
}

type countingSpanExporter struct {
	exported atomic.Int64
}

func (e *countingSpanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.exported.Add(int64(len(spans)))
	return nil
}

func (*countingSpanExporter) Shutdown(context.Context) error { return nil }

type failingSpanExporter struct{}

func (*failingSpanExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	return errors.New("collector unavailable")
}

func (*failingSpanExporter) Shutdown(context.Context) error { return nil }

type blockingSpanExporter struct{}

func (*blockingSpanExporter) ExportSpans(ctx context.Context, _ []sdktrace.ReadOnlySpan) error {
	<-ctx.Done()
	return ctx.Err()
}

func (*blockingSpanExporter) Shutdown(context.Context) error { return nil }

func TestForceFlushExportsQueuedSpans(t *testing.T) {
	exporter := &countingSpanExporter{}
	batchOptions, err := tracingimpl.BatchOptions(tracingimpl.Config{BatchTimeout: time.Hour})
	if err != nil {
		t.Fatalf("tracingimpl.BatchOptions() error = %v", err)
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter, batchOptions...))
	logger := setupLog(WithOutput(&discardWriter{}))
	logger.telemetry.Lifecycle.TracerProvider = provider
	logger.telemetry.Tracer = provider.Tracer(instrumentationName)
	t.Cleanup(func() { _ = logger.Shutdown(context.Background()) })

	ctx, end := logger.StartTrace(context.Background(), "queued-span")
	if !sdktraceReadOnlySpanContext(ctx) {
		t.Fatal("StartTrace() did not create a valid span")
	}
	end()
	if err := logger.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() error = %v", err)
	}
	if spans := exporter.exported.Load(); spans != 1 {
		t.Fatalf("exported spans = %d, want 1", spans)
	}
}

func sdktraceReadOnlySpanContext(ctx context.Context) bool {
	return trace.SpanContextFromContext(ctx).IsValid()
}

func TestShutdownLifecycle(t *testing.T) {
	t.Run("multiple shutdown calls are safe", func(t *testing.T) {
		provider := sdktrace.NewTracerProvider()
		logger := setupLog(WithOutput(&discardWriter{}))
		logger.telemetry.Lifecycle.TracerProvider = provider
		logger.telemetry.Tracer = provider.Tracer(instrumentationName)

		if err := logger.Shutdown(context.Background()); err != nil {
			t.Fatalf("first Shutdown() error = %v", err)
		}
		if err := logger.Shutdown(context.Background()); err != nil {
			t.Fatalf("second Shutdown() error = %v", err)
		}
		if err := logger.ForceFlush(context.Background()); !errors.Is(err, ErrTracingShutdown) {
			t.Errorf("ForceFlush() error = %v, want ErrTracingShutdown", err)
		}
		ctx, end := logger.StartTrace(context.Background(), "after-shutdown")
		defer end()
		if trace.SpanContextFromContext(ctx).IsValid() {
			t.Error("StartTrace() created a span after shutdown")
		}
	})

	t.Run("shutdown returns context timeout", func(t *testing.T) {
		batchOptions, err := tracingimpl.BatchOptions(tracingimpl.Config{BatchTimeout: time.Hour})
		if err != nil {
			t.Fatalf("tracingimpl.BatchOptions() error = %v", err)
		}
		provider := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(&blockingSpanExporter{}, batchOptions...),
		)
		logger := setupLog(WithOutput(&discardWriter{}))
		logger.telemetry.Lifecycle.TracerProvider = provider
		logger.telemetry.Tracer = provider.Tracer(instrumentationName)
		_, span := logger.telemetry.Tracer.Start(context.Background(), "blocking-span")
		span.End()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if err := logger.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Shutdown() error = %v, want context deadline exceeded", err)
		}
	})
}
