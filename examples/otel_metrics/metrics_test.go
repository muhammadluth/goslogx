package otelmetrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhammadluth/goslogx"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestHTTPREDMetricsUsesRoutePatternAndRequestContext(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	logger := goslogx.New(
		goslogx.WithOutput(io.Discard),
		goslogx.WithMeterProvider(provider),
	)
	metrics, err := NewHTTPREDMetrics(logger, "payment-api", func(*http.Request) string {
		return "/payments/{id}"
	})
	if err != nil {
		t.Fatalf("NewHTTPREDMetrics() error = %v", err)
	}

	handler := metrics.Middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	request := httptest.NewRequest(http.MethodGet, "https://example.test/payments/pay-123", nil)
	handler.ServeHTTP(httptest.NewRecorder(), request)
	metrics.RecordPaymentCreated(request.Context())

	collected := collectMetrics(t, reader)
	assertMetricSum(t, collected, "http.server.requests", 1, map[string]string{
		"http.request.method": http.MethodGet,
		"http.route":          "/payments/{id}",
	})
	assertMetricSum(t, collected, "http.server.errors", 1, map[string]string{
		"http.request.method": http.MethodGet,
		"http.route":          "/payments/{id}",
	})
	assertHistogramCount(t, collected, "http.server.duration", 1, map[string]string{
		"http.request.method": http.MethodGet,
		"http.route":          "/payments/{id}",
	})
	assertMetricSum(t, collected, "payment.events", 1, map[string]string{
		"event.name": paymentCreatedEvent,
	})
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()

	var resourceMetrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &resourceMetrics); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	metrics := make(map[string]metricdata.Metrics)
	for _, scope := range resourceMetrics.ScopeMetrics {
		for _, data := range scope.Metrics {
			metrics[data.Name] = data
		}
	}
	return metrics
}

func assertMetricSum(t *testing.T, metrics map[string]metricdata.Metrics, name string, want int64, attributes map[string]string) {
	t.Helper()

	data, ok := metrics[name]
	if !ok {
		t.Fatalf("metric %q was not collected", name)
	}
	sum, ok := data.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("metric %q data type = %T, want metricdata.Sum[int64]", name, data.Data)
	}
	var got int64
	for _, point := range sum.DataPoints {
		if matchesAttributes(point.Attributes, attributes) {
			got += point.Value
		}
	}
	if got != want {
		t.Errorf("metric %q value = %d, want %d", name, got, want)
	}
}

func assertHistogramCount(t *testing.T, metrics map[string]metricdata.Metrics, name string, want uint64, attributes map[string]string) {
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
		if matchesAttributes(point.Attributes, attributes) {
			got += point.Count
		}
	}
	if got != want {
		t.Errorf("metric %q count = %d, want %d", name, got, want)
	}
}

func matchesAttributes(set attribute.Set, want map[string]string) bool {
	if set.Len() != len(want) {
		return false
	}
	for key, value := range want {
		actual, ok := set.Value(attribute.Key(key))
		if !ok || actual.AsString() != value {
			return false
		}
	}
	return true
}
