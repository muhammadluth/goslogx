package goslogx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
)

type shutdownErrorSpanExporter struct{}

func (*shutdownErrorSpanExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	return nil
}

func (*shutdownErrorSpanExporter) Shutdown(context.Context) error {
	return errors.New("span exporter shutdown failed")
}

func TestInternalMetricsDisabled(t *testing.T) {
	logger, reader := newInternalMetricsTestLogger(t, false)

	logger.InfoContext(
		context.Background(),
		"request-sensitive",
		"dynamic-module",
		MESSSAGE_TYPE_EVENT,
		"sensitive message",
		nil,
	)
	if err := logger.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() error = %v", err)
	}
	if err := logger.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	metrics := collectInternalMetrics(t, reader)
	if len(metrics) != 0 {
		t.Fatalf("internal metrics disabled, collected %d metrics", len(metrics))
	}
}

func TestInternalMetricsRecordsBoundedSignals(t *testing.T) {
	logger, reader := newInternalMetricsTestLogger(t, true, WithDebug(true))
	ctx := context.Background()

	logger.Debug("trace-debug", "module-debug", MESSSAGE_TYPE_EVENT, "debug message", nil)
	logger.InfoContext(ctx, "trace-info", "module-info", MESSSAGE_TYPE_EVENT, "info message", map[string]string{
		"url": "https://example.test/payments/payment-123",
	})
	logger.Info("trace-info-2", "module-info-2", MESSSAGE_TYPE_EVENT, "info message 2", nil)
	logger.Warning("trace-warning", "module-warning", "warning message", nil)
	logger.ErrorContext(ctx, "trace-error", "module-error", errors.New("secret error text"))
	logger.recordLogMetric(ctx, severityCritical, zapcore.FatalLevel)

	handler := newRateLimitedOTelErrorHandler(logger.logger, DefaultOTelErrorLogInterval, logger.internalMetrics)
	handler.Handle(errors.New("collector credential rejected"))
	handler.Handle(errors.New("collector credential rejected again"))

	if err := logger.ForceFlush(ctx); err != nil {
		t.Fatalf("ForceFlush() error = %v", err)
	}
	if err := logger.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	metrics := collectInternalMetrics(t, reader)
	logRecords := requireInt64Sum(t, metrics, metricLogRecords)
	wantSeverityCounts := map[string]int64{
		severityDebug:    1,
		severityInfo:     2,
		severityWarning:  1,
		severityError:    1,
		severityCritical: 1,
	}
	for severity, want := range wantSeverityCounts {
		if got := sumForAttribute(logRecords, "severity", severity); got != want {
			t.Errorf("%s severity count = %d, want %d", severity, got, want)
		}
	}

	telemetryErrors := requireInt64Sum(t, metrics, metricTelemetryErrors)
	if got := sumDataPoints(telemetryErrors.DataPoints); got != 2 {
		t.Errorf("telemetry error count = %d, want 2", got)
	}
	requireHistogramCount(t, metrics, metricForceFlushDuration, 1)
	requireHistogramCount(t, metrics, metricShutdownDuration, 1)
	assertBoundedMetricAttributes(t, metrics)
}

func TestInternalMetricAttributesUseStrictAllowlist(t *testing.T) {
	logger, reader := newInternalMetricsTestLogger(t, true)
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)

	const dynamicValues = 32
	for index := 0; index < dynamicValues; index++ {
		traceID := fmt.Sprintf("trace-%d", index)
		module := fmt.Sprintf("module-%d", index)
		logger.InfoContext(
			ctx,
			traceID,
			module,
			MESSSAGE_TYPE_EVENT,
			fmt.Sprintf("message-%d", index),
			map[string]string{"url": fmt.Sprintf("/payments/payment-%d", index)},
		)
		logger.ErrorContext(ctx, traceID, module, fmt.Errorf("private error text %d", index))
	}

	metrics := collectInternalMetrics(t, reader)
	logRecords := requireInt64Sum(t, metrics, metricLogRecords)
	if got := len(logRecords.DataPoints); got != 2 {
		t.Fatalf("log metric series = %d, want 2 bounded severity series", got)
	}
	if got := sumForAttribute(logRecords, "severity", severityInfo); got != dynamicValues {
		t.Errorf("info count = %d, want %d", got, dynamicValues)
	}
	if got := sumForAttribute(logRecords, "severity", severityError); got != dynamicValues {
		t.Errorf("error count = %d, want %d", got, dynamicValues)
	}
	assertBoundedMetricAttributes(t, metrics)
}

func TestInternalMetricsRecordsLifecycleErrors(t *testing.T) {
	logger, reader := newInternalMetricsTestLogger(t, true)

	flushProvider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = flushProvider.Shutdown(context.Background()) })
	logger.tracerProvider = flushProvider
	logger.tracingClosed.Store(true)
	if err := logger.ForceFlush(context.Background()); !errors.Is(err, ErrTracingShutdown) {
		t.Fatalf("ForceFlush() error = %v, want ErrTracingShutdown", err)
	}

	shutdownProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(&shutdownErrorSpanExporter{}))
	logger.tracerProvider = shutdownProvider
	logger.tracingClosed.Store(false)
	if err := logger.Shutdown(context.Background()); err == nil {
		t.Fatal("Shutdown() error = nil, want exporter shutdown error")
	}

	metrics := collectInternalMetrics(t, reader)
	if got := sumDataPoints(requireInt64Sum(t, metrics, metricForceFlushErrors).DataPoints); got != 1 {
		t.Errorf("force flush error count = %d, want 1", got)
	}
	if got := sumDataPoints(requireInt64Sum(t, metrics, metricShutdownErrors).DataPoints); got != 1 {
		t.Errorf("shutdown error count = %d, want 1", got)
	}
	requireHistogramCount(t, metrics, metricForceFlushDuration, 1)
	requireHistogramCount(t, metrics, metricShutdownDuration, 1)
	assertBoundedMetricAttributes(t, metrics)
}

func newInternalMetricsTestLogger(t *testing.T, enabled bool, opts ...Option) (*Logger, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	loggerOptions := []Option{
		WithOutput(&discardWriter{}),
		WithMeterProvider(provider),
		WithInternalMetrics(enabled),
	}
	loggerOptions = append(loggerOptions, opts...)
	return setupLog(loggerOptions...), reader
}

func collectInternalMetrics(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()

	var resourceMetrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &resourceMetrics); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	metrics := make(map[string]metricdata.Metrics)
	for _, scope := range resourceMetrics.ScopeMetrics {
		if scope.Scope.Name != instrumentationName {
			continue
		}
		for _, data := range scope.Metrics {
			metrics[data.Name] = data
		}
	}
	return metrics
}

func requireInt64Sum(t *testing.T, metrics map[string]metricdata.Metrics, name string) metricdata.Sum[int64] {
	t.Helper()

	data, ok := metrics[name]
	if !ok {
		t.Fatalf("metric %q was not collected", name)
	}
	sum, ok := data.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("metric %q data type = %T, want metricdata.Sum[int64]", name, data.Data)
	}
	return sum
}

func requireHistogramCount(t *testing.T, metrics map[string]metricdata.Metrics, name string, want uint64) {
	t.Helper()

	data, ok := metrics[name]
	if !ok {
		t.Fatalf("metric %q was not collected", name)
	}
	histogram, ok := data.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("metric %q data type = %T, want metricdata.Histogram[float64]", name, data.Data)
	}
	var got uint64
	for _, point := range histogram.DataPoints {
		got += point.Count
	}
	if got != want {
		t.Errorf("metric %q histogram count = %d, want %d", name, got, want)
	}
}

func sumDataPoints(points []metricdata.DataPoint[int64]) int64 {
	var total int64
	for _, point := range points {
		total += point.Value
	}
	return total
}

func sumForAttribute(sum metricdata.Sum[int64], key, value string) int64 {
	var total int64
	for _, point := range sum.DataPoints {
		attributeValue, ok := point.Attributes.Value(attribute.Key(key))
		if ok && attributeValue.AsString() == value {
			total += point.Value
		}
	}
	return total
}

func assertBoundedMetricAttributes(t *testing.T, metrics map[string]metricdata.Metrics) {
	t.Helper()

	allowedValues := map[attribute.Key]map[string]struct{}{
		"severity": {
			severityDebug:    {},
			severityInfo:     {},
			severityWarning:  {},
			severityError:    {},
			severityCritical: {},
		},
		"operation": {
			operationErrorHandler: {},
			operationForceFlush:   {},
			operationShutdown:     {},
		},
		"result": {
			resultSuccess: {},
			resultError:   {},
		},
	}
	for name, data := range metrics {
		switch aggregation := data.Data.(type) {
		case metricdata.Sum[int64]:
			for _, point := range aggregation.DataPoints {
				assertAllowedAttributeSet(t, name, point.Attributes, allowedValues)
			}
		case metricdata.Histogram[float64]:
			for _, point := range aggregation.DataPoints {
				assertAllowedAttributeSet(t, name, point.Attributes, allowedValues)
			}
		default:
			t.Errorf("metric %q has unexpected aggregation %T", name, data.Data)
		}
	}
}

func assertAllowedAttributeSet(t *testing.T, metricName string, set attribute.Set, allowed map[attribute.Key]map[string]struct{}) {
	t.Helper()

	for _, keyValue := range set.ToSlice() {
		allowedForKey, ok := allowed[keyValue.Key]
		if !ok {
			t.Errorf("metric %q contains forbidden attribute %q", metricName, keyValue.Key)
			continue
		}
		value := keyValue.Value.Emit()
		if _, ok := allowedForKey[value]; !ok {
			t.Errorf("metric %q attribute %q contains non-allowlisted value %q", metricName, keyValue.Key, value)
		}
		for _, sensitive := range []string{"trace-", "module-", "message", "payment-123", "credential", "secret"} {
			if strings.Contains(value, sensitive) {
				t.Errorf("metric %q attribute %q leaked dynamic value %q", metricName, keyValue.Key, value)
			}
		}
	}
}

var _ sdktrace.SpanExporter = (*shutdownErrorSpanExporter)(nil)
var _ trace.TracerProvider = (*sdktrace.TracerProvider)(nil)
