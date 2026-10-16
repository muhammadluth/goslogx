package goslogx

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type lifecycleMetricExporter struct {
	exports    atomic.Int64
	forceFlush atomic.Int64
	shutdown   atomic.Int64
}

func (e *lifecycleMetricExporter) Temporality(sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (e *lifecycleMetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

func (e *lifecycleMetricExporter) Export(context.Context, *metricdata.ResourceMetrics) error {
	e.exports.Add(1)
	return nil
}

func (e *lifecycleMetricExporter) ForceFlush(context.Context) error {
	e.forceFlush.Add(1)
	return nil
}

func (e *lifecycleMetricExporter) Shutdown(context.Context) error {
	e.shutdown.Add(1)
	return nil
}

func TestMeterProviderDependencyInjection(t *testing.T) {
	provider := sdkmetric.NewMeterProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	globalBefore := otel.GetMeterProvider()
	logger := setupLog(
		WithOutput(&discardWriter{}),
		WithMeterProvider(provider),
	)

	if logger.MeterProvider() != provider {
		t.Fatal("MeterProvider() did not return the external provider")
	}
	if otel.GetMeterProvider() != globalBefore {
		t.Fatal("WithMeterProvider replaced the global provider")
	}
}

func TestGlobalMeterProviderUsesGlobalLogger(t *testing.T) {
	provider := sdkmetric.NewMeterProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	previous := globalLog.Load()
	globalLog.Store(setupLog(
		WithOutput(&discardWriter{}),
		WithMeterProvider(provider),
	))
	t.Cleanup(func() { globalLog.Store(previous) })

	if MeterProvider() != provider {
		t.Fatal("global MeterProvider() did not return the global logger provider")
	}
}

func TestNilMeterProviderUsesNoopProvider(t *testing.T) {
	logger := setupLog(
		WithOutput(&discardWriter{}),
		WithMeterProvider(nil),
	)

	provider := logger.MeterProvider()
	if provider == nil {
		t.Fatal("MeterProvider() returned nil")
	}

	counter, err := provider.Meter("goslogx-test").Int64Counter("test.counter")
	if err != nil {
		t.Fatalf("Int64Counter() error = %v", err)
	}
	counter.Add(context.Background(), 1)
}

func TestExternalMeterProviderLifecycle(t *testing.T) {
	exporter := &lifecycleMetricExporter{}
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			exporter,
			sdkmetric.WithInterval(time.Hour),
		)),
	)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	previousGlobal := otel.GetMeterProvider()
	globalProvider := sdkmetric.NewMeterProvider()
	otel.SetMeterProvider(globalProvider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previousGlobal)
		_ = globalProvider.Shutdown(context.Background())
	})

	logger := setupLog(
		WithOutput(&discardWriter{}),
		WithMeterProvider(provider),
	)
	if otel.GetMeterProvider() != globalProvider {
		t.Fatal("WithMeterProvider replaced the global MeterProvider")
	}

	counter, err := provider.Meter("goslogx-test").Int64Counter("test.counter")
	if err != nil {
		t.Fatalf("Int64Counter() error = %v", err)
	}
	counter.Add(context.Background(), 1)

	if err := logger.ForceFlush(context.Background()); err != nil {
		t.Fatalf("logger.ForceFlush() error = %v", err)
	}
	if got := exporter.forceFlush.Load(); got != 0 {
		t.Fatalf("external MeterProvider force flush calls = %d, want 0", got)
	}

	if err := logger.Shutdown(context.Background()); err != nil {
		t.Fatalf("logger.Shutdown() error = %v", err)
	}
	if got := exporter.shutdown.Load(); got != 0 {
		t.Fatalf("external MeterProvider shutdown calls = %d, want 0", got)
	}

	counter.Add(context.Background(), 1)
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("external MeterProvider ForceFlush() error = %v", err)
	}
	if got := exporter.forceFlush.Load(); got != 1 {
		t.Fatalf("external MeterProvider force flush calls = %d, want 1", got)
	}
	if got := exporter.exports.Load(); got == 0 {
		t.Fatal("external MeterProvider did not export metrics after logger shutdown")
	}
}

var _ metric.MeterProvider = (*sdkmetric.MeterProvider)(nil)
var _ sdkmetric.Exporter = (*lifecycleMetricExporter)(nil)
