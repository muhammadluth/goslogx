package goslogx

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
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

func TestBatchProcessorShutdownFlushesQueuedSpans(t *testing.T) {
	exporter := &countingSpanExporter{}
	batchOptions, err := newBatchProcessorOptions(TracingConfig{
		BatchTimeout: time.Hour,
	})
	if err != nil {
		t.Fatalf("newBatchProcessorOptions() error = %v", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, batchOptions...),
	)

	_, span := provider.Tracer(instrumentationName).Start(context.Background(), "queued-span")
	span.End()
	if spans := exporter.exported.Load(); spans != 0 {
		t.Fatalf("spans exported before shutdown = %d, want 0", spans)
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if spans := exporter.exported.Load(); spans != 1 {
		t.Fatalf("spans exported after shutdown = %d, want 1", spans)
	}
}

func TestBatchProcessorLightLoadDoesNotBlock(t *testing.T) {
	exporter := &countingSpanExporter{}
	batchOptions, err := newBatchProcessorOptions(TracingConfig{})
	if err != nil {
		t.Fatalf("newBatchProcessorOptions() error = %v", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, batchOptions...),
	)
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		tracer := provider.Tracer(instrumentationName)
		for range 100 {
			_, span := tracer.Start(context.Background(), "light-load")
			span.End()
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ending spans blocked under light load")
	}
}

func TestForceFlushExportsQueuedSpans(t *testing.T) {
	exporter := &countingSpanExporter{}
	batchOptions, err := newBatchProcessorOptions(TracingConfig{BatchTimeout: time.Hour})
	if err != nil {
		t.Fatalf("newBatchProcessorOptions() error = %v", err)
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter, batchOptions...))
	logger := setupLog(WithOutput(&discardWriter{}))
	logger.tracerProvider = provider
	logger.tracer = provider.Tracer(instrumentationName)
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
		logger.tracerProvider = provider
		logger.tracer = provider.Tracer(instrumentationName)

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
		batchOptions, err := newBatchProcessorOptions(TracingConfig{BatchTimeout: time.Hour})
		if err != nil {
			t.Fatalf("newBatchProcessorOptions() error = %v", err)
		}
		provider := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(&blockingSpanExporter{}, batchOptions...),
		)
		logger := setupLog(WithOutput(&discardWriter{}))
		logger.tracerProvider = provider
		logger.tracer = provider.Tracer(instrumentationName)
		_, span := logger.tracer.Start(context.Background(), "blocking-span")
		span.End()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if err := logger.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Shutdown() error = %v, want context deadline exceeded", err)
		}
	})
}

func TestOTelExportErrorsAreRateLimited(t *testing.T) {
	var output lockedBuffer
	logger := setupLog(WithOutput(&output))
	handler := newRateLimitedOTelErrorHandler(logger.logger, time.Hour, nil)
	previousHandler := otel.GetErrorHandler()
	otel.SetErrorHandler(handler)
	t.Cleanup(func() { otel.SetErrorHandler(previousHandler) })

	batchOptions, err := newBatchProcessorOptions(TracingConfig{
		BatchTimeout:       time.Hour,
		MaxExportBatchSize: 1,
	})
	if err != nil {
		t.Fatalf("newBatchProcessorOptions() error = %v", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(&failingSpanExporter{}, batchOptions...),
	)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	tracer := provider.Tracer(instrumentationName)
	for range 2 {
		_, span := tracer.Start(context.Background(), "failed-export")
		span.End()
	}

	deadline := time.Now().Add(time.Second)
	for output.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if output.Len() == 0 {
		t.Fatal("export failure was not logged")
	}
	for time.Now().Before(deadline) {
		handler.mu.Lock()
		suppressed := handler.suppressed
		handler.mu.Unlock()
		if suppressed > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	handler.mu.Lock()
	suppressed := handler.suppressed
	handler.mu.Unlock()
	if suppressed == 0 {
		t.Fatal("repeated export error was not suppressed")
	}
	loggedOutput := output.String()
	if lines := strings.Count(strings.TrimSpace(loggedOutput), "\n") + 1; lines != 1 {
		t.Errorf("error log lines = %d, want 1; output: %s", lines, loggedOutput)
	}
}
