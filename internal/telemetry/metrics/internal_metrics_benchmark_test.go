package metrics

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func BenchmarkInternalMetricCounterRecording(b *testing.B) {
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
	b.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	recorder := NewRecorder(provider)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		recorder.RecordLog(ctx, severityInfo)
	}
}
func BenchmarkInternalMetricHistogramRecording(b *testing.B) {
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
	b.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	recorder := NewRecorder(provider)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		recorder.RecordForceFlush(ctx, time.Millisecond, nil)
	}
}
