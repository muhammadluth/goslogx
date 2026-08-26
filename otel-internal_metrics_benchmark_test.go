package goslogx

import (
	"context"
	"errors"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func BenchmarkInfoContextMetricsDisabled(b *testing.B) {
	logger := newInternalMetricsBenchmarkLogger(b, false)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		logger.InfoContext(ctx, "request-123", "payment", MESSSAGE_TYPE_EVENT, "request received", nil)
	}
}

func BenchmarkInfoContextMetricsEnabled(b *testing.B) {
	logger := newInternalMetricsBenchmarkLogger(b, true)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		logger.InfoContext(ctx, "request-123", "payment", MESSSAGE_TYPE_EVENT, "request received", nil)
	}
}

func BenchmarkErrorContextMetricsDisabled(b *testing.B) {
	logger := newInternalMetricsBenchmarkLogger(b, false)
	ctx := context.Background()
	err := errors.New("payment rejected")
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		logger.ErrorContext(ctx, "request-123", "payment", err)
	}
}

func BenchmarkErrorContextMetricsEnabled(b *testing.B) {
	logger := newInternalMetricsBenchmarkLogger(b, true)
	ctx := context.Background()
	err := errors.New("payment rejected")
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		logger.ErrorContext(ctx, "request-123", "payment", err)
	}
}

func BenchmarkInternalMetricCounterRecording(b *testing.B) {
	logger := newInternalMetricsBenchmarkLogger(b, true)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		logger.internalMetrics.recordLog(ctx, severityInfo)
	}
}

func BenchmarkInternalMetricHistogramRecording(b *testing.B) {
	logger := newInternalMetricsBenchmarkLogger(b, true)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		logger.internalMetrics.recordForceFlush(ctx, time.Millisecond, nil)
	}
}

func newInternalMetricsBenchmarkLogger(b *testing.B, enabled bool) *Logger {
	b.Helper()

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
	b.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return setupLog(
		WithOutput(&discardWriter{}),
		WithMeterProvider(provider),
		WithInternalMetrics(enabled),
	)
}
