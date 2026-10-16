package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tracingimpl "github.com/muhammadluth/goslogx/internal/telemetry/tracing"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestOTelExportErrorsAreRateLimited(t *testing.T) {
	var output lockedBuffer
	logger := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.DebugLevel))
	handler := NewErrorHandler(logger, time.Hour, nil)
	previousHandler := otel.GetErrorHandler()
	otel.SetErrorHandler(handler)
	t.Cleanup(func() { otel.SetErrorHandler(previousHandler) })
	batchOptions, err := tracingimpl.BatchOptions(tracingimpl.Config{
		BatchTimeout:       time.Hour,
		MaxExportBatchSize: 1,
	})
	if err != nil {
		t.Fatalf("tracingimpl.BatchOptions() error = %v", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(&failingSpanExporter{}, batchOptions...),
	)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	tracer := provider.Tracer(tracingimpl.ScopeName)
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

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}
func (b *lockedBuffer) Len() int       { b.mu.Lock(); defer b.mu.Unlock(); return b.buffer.Len() }
func (b *lockedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buffer.String() }

type failingSpanExporter struct{}

func (*failingSpanExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	return errors.New("collector unavailable")
}
func (*failingSpanExporter) Shutdown(context.Context) error { return nil }
