package tracing

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestBatchProcessorShutdownFlushesQueuedSpans(t *testing.T) {
	exporter := &countingSpanExporter{}
	batchOptions, err := BatchOptions(Config{BatchTimeout: time.Hour})
	if err != nil {
		t.Fatalf("BatchOptions() error = %v", err)
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter, batchOptions...))
	_, span := provider.Tracer(ScopeName).Start(context.Background(), "queued-span")
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
	batchOptions, err := BatchOptions(Config{})
	if err != nil {
		t.Fatalf("BatchOptions() error = %v", err)
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
		tracer := provider.Tracer(ScopeName)
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

type countingSpanExporter struct{ exported atomic.Int64 }

func (e *countingSpanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.exported.Add(int64(len(spans)))
	return nil
}
func (*countingSpanExporter) Shutdown(context.Context) error { return nil }
