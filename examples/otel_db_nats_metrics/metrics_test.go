package oteldbnatsmetrics

import (
	"context"
	"database/sql"
	"io"
	"testing"
	"time"

	"github.com/muhammadluth/goslogx"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestDBPoolAndNATSMetricsUseBoundedAttributes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	logger := goslogx.New(
		goslogx.WithOutput(io.Discard),
		goslogx.WithMeterProvider(provider),
	)
	dbObserver, err := NewDBPoolObserver(logger, "payment-api/db", &sql.DB{}, "payments-primary")
	if err != nil {
		t.Fatalf("NewDBPoolObserver() error = %v", err)
	}
	dbObserver.observeStats(context.Background(), sql.DBStats{
		OpenConnections: 8,
		InUse:           5,
		Idle:            3,
		WaitCount:       2,
		WaitDuration:    time.Second,
	})
	dbObserver.observeStats(context.Background(), sql.DBStats{
		OpenConnections: 10,
		InUse:           6,
		Idle:            4,
		WaitCount:       5,
		WaitDuration:    3 * time.Second,
	})

	natsMetrics, err := NewNATSMetrics(logger, "payment-api/nats", "payments", "payment-worker")
	if err != nil {
		t.Fatalf("NewNATSMetrics() error = %v", err)
	}
	natsMetrics.ObserveConsumer(context.Background(), NATSConsumerSnapshot{
		PendingMessages:     12,
		AckPending:          3,
		RedeliveredMessages: 1,
		Lag:                 9,
	})
	natsMetrics.RecordPublish(context.Background(), 10*time.Millisecond)
	natsMetrics.RecordConsume(context.Background(), 20*time.Millisecond)

	metrics := collectDBNATSMetrics(t, reader)
	assertGaugeValue(t, metrics, "db.client.connections.open", 10, map[string]string{
		"db.client.connection.pool.name": "payments-primary",
	})
	assertGaugeValue(t, metrics, "db.client.connections.in_use", 6, map[string]string{
		"db.client.connection.pool.name": "payments-primary",
	})
	assertGaugeValue(t, metrics, "db.client.connections.idle", 4, map[string]string{
		"db.client.connection.pool.name": "payments-primary",
	})
	assertSumValue(t, metrics, "db.client.connections.wait_count", 5, map[string]string{
		"db.client.connection.pool.name": "payments-primary",
	})
	assertFloatSumValue(t, metrics, "db.client.connections.wait_duration", 3, map[string]string{
		"db.client.connection.pool.name": "payments-primary",
	})
	assertGaugeValue(t, metrics, "messaging.consumer.pending_messages", 12, map[string]string{
		"messaging.system":           "nats",
		"messaging.destination.name": "payments",
		"messaging.consumer.name":    "payment-worker",
	})
	assertGaugeValue(t, metrics, "messaging.consumer.ack_pending", 3, map[string]string{
		"messaging.system":           "nats",
		"messaging.destination.name": "payments",
		"messaging.consumer.name":    "payment-worker",
	})
	assertGaugeValue(t, metrics, "messaging.consumer.redelivered_messages", 1, map[string]string{
		"messaging.system":           "nats",
		"messaging.destination.name": "payments",
		"messaging.consumer.name":    "payment-worker",
	})
	assertSumValue(t, metrics, "messaging.publish.messages", 1, map[string]string{
		"messaging.system":           "nats",
		"messaging.destination.name": "payments",
	})
	assertSumValue(t, metrics, "messaging.consume.messages", 1, map[string]string{
		"messaging.system":           "nats",
		"messaging.destination.name": "payments",
		"messaging.consumer.name":    "payment-worker",
	})
	assertGaugeValue(t, metrics, "messaging.consumer.lag", 9, map[string]string{
		"messaging.system":           "nats",
		"messaging.destination.name": "payments",
		"messaging.consumer.name":    "payment-worker",
	})
	assertHistogramCount(t, metrics, "messaging.operation.duration", 2)
	assertNoMessageIDAttributes(t, metrics)
}

func collectDBNATSMetrics(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
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

func assertGaugeValue(t *testing.T, metrics map[string]metricdata.Metrics, name string, want int64, attributes map[string]string) {
	t.Helper()

	data, ok := metrics[name]
	if !ok {
		t.Fatalf("metric %q was not collected", name)
	}
	gauge, ok := data.Data.(metricdata.Gauge[int64])
	if !ok {
		t.Fatalf("metric %q data type = %T, want metricdata.Gauge[int64]", name, data.Data)
	}
	for _, point := range gauge.DataPoints {
		if matchesDBNATSAttributes(point.Attributes, attributes) {
			if point.Value != want {
				t.Errorf("metric %q value = %d, want %d", name, point.Value, want)
			}
			return
		}
	}
	t.Errorf("metric %q did not contain expected attributes", name)
}

func assertSumValue(t *testing.T, metrics map[string]metricdata.Metrics, name string, want int64, attributes map[string]string) {
	t.Helper()

	data, ok := metrics[name]
	if !ok {
		t.Fatalf("metric %q was not collected", name)
	}
	sum, ok := data.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("metric %q data type = %T, want metricdata.Sum[int64]", name, data.Data)
	}
	for _, point := range sum.DataPoints {
		if matchesDBNATSAttributes(point.Attributes, attributes) {
			if point.Value != want {
				t.Errorf("metric %q value = %d, want %d", name, point.Value, want)
			}
			return
		}
	}
	t.Errorf("metric %q did not contain expected attributes", name)
}

func assertFloatSumValue(t *testing.T, metrics map[string]metricdata.Metrics, name string, want float64, attributes map[string]string) {
	t.Helper()

	data, ok := metrics[name]
	if !ok {
		t.Fatalf("metric %q was not collected", name)
	}
	sum, ok := data.Data.(metricdata.Sum[float64])
	if !ok {
		t.Fatalf("metric %q data type = %T, want metricdata.Sum[float64]", name, data.Data)
	}
	for _, point := range sum.DataPoints {
		if matchesDBNATSAttributes(point.Attributes, attributes) {
			if point.Value != want {
				t.Errorf("metric %q value = %v, want %v", name, point.Value, want)
			}
			return
		}
	}
	t.Errorf("metric %q did not contain expected attributes", name)
}

func assertHistogramCount(t *testing.T, metrics map[string]metricdata.Metrics, name string, want uint64) {
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
		t.Errorf("metric %q count = %d, want %d", name, got, want)
	}
}

func matchesDBNATSAttributes(set attribute.Set, want map[string]string) bool {
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

func assertNoMessageIDAttributes(t *testing.T, metrics map[string]metricdata.Metrics) {
	t.Helper()

	for metricName, data := range metrics {
		switch aggregation := data.Data.(type) {
		case metricdata.Gauge[int64]:
			for _, point := range aggregation.DataPoints {
				assertNoMessageIDAttribute(t, metricName, point.Attributes)
			}
		case metricdata.Sum[int64]:
			for _, point := range aggregation.DataPoints {
				assertNoMessageIDAttribute(t, metricName, point.Attributes)
			}
		case metricdata.Sum[float64]:
			for _, point := range aggregation.DataPoints {
				assertNoMessageIDAttribute(t, metricName, point.Attributes)
			}
		case metricdata.Histogram[float64]:
			for _, point := range aggregation.DataPoints {
				assertNoMessageIDAttribute(t, metricName, point.Attributes)
			}
		default:
			t.Errorf("metric %q has unexpected aggregation %T", metricName, data.Data)
		}
	}
}

func assertNoMessageIDAttribute(t *testing.T, metricName string, set attribute.Set) {
	t.Helper()
	for _, keyValue := range set.ToSlice() {
		if keyValue.Key == "messaging.message.id" || keyValue.Key == "message.id" {
			t.Errorf("metric %q contains forbidden message ID attribute", metricName)
		}
	}
}
