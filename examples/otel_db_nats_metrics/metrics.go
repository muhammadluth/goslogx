// Package oteldbnatsmetrics contains copy-and-adapt database and NATS metric
// examples. It deliberately lives outside goslogx so the logging library does
// not depend on database/sql drivers or NATS clients.
package oteldbnatsmetrics

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/muhammadluth/goslogx"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// DBPoolObserver records low-cardinality connection-pool measurements from an
// application-owned *sql.DB. Call Observe on an application-owned interval;
// goslogx neither stores nor manages the database handle.
type DBPoolObserver struct {
	db         *sql.DB
	attributes []attribute.KeyValue

	openConnections  metric.Int64Gauge
	inUseConnections metric.Int64Gauge
	idleConnections  metric.Int64Gauge
	waitCount        metric.Int64Counter
	waitDuration     metric.Float64Counter

	mu               sync.Mutex
	previousWaits    int64
	previousWaitTime time.Duration
}

// NewDBPoolObserver creates instruments for an application-owned SQL pool.
// poolName must be a stable name, not a per-request value or connection string
// containing credentials.
func NewDBPoolObserver(logger *goslogx.Logger, instrumentationScope string, db *sql.DB, poolName string) (*DBPoolObserver, error) {
	if logger == nil {
		return nil, errors.New("otel db metrics: logger is required")
	}
	if instrumentationScope == "" {
		return nil, errors.New("otel db metrics: instrumentation scope is required")
	}
	if db == nil {
		return nil, errors.New("otel db metrics: database is required")
	}
	if poolName == "" {
		return nil, errors.New("otel db metrics: pool name is required")
	}

	meter := logger.MeterProvider().Meter(instrumentationScope)
	openConnections, err := meter.Int64Gauge("db.client.connections.open", metric.WithUnit("{connection}"))
	if err != nil {
		return nil, err
	}
	inUseConnections, err := meter.Int64Gauge("db.client.connections.in_use", metric.WithUnit("{connection}"))
	if err != nil {
		return nil, err
	}
	idleConnections, err := meter.Int64Gauge("db.client.connections.idle", metric.WithUnit("{connection}"))
	if err != nil {
		return nil, err
	}
	waitCount, err := meter.Int64Counter("db.client.connections.wait_count", metric.WithUnit("{request}"))
	if err != nil {
		return nil, err
	}
	waitDuration, err := meter.Float64Counter("db.client.connections.wait_duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}

	return &DBPoolObserver{
		db:               db,
		attributes:       []attribute.KeyValue{attribute.String("db.client.connection.pool.name", poolName)},
		openConnections:  openConnections,
		inUseConnections: inUseConnections,
		idleConnections:  idleConnections,
		waitCount:        waitCount,
		waitDuration:     waitDuration,
	}, nil
}

// Observe records a snapshot of the application-owned SQL connection pool.
func (o *DBPoolObserver) Observe(ctx context.Context) {
	if o == nil || o.db == nil {
		return
	}
	o.observeStats(ctx, o.db.Stats())
}

func (o *DBPoolObserver) observeStats(ctx context.Context, stats sql.DBStats) {
	if o == nil {
		return
	}

	o.openConnections.Record(ctx, int64(stats.OpenConnections), metric.WithAttributes(o.attributes...))
	o.inUseConnections.Record(ctx, int64(stats.InUse), metric.WithAttributes(o.attributes...))
	o.idleConnections.Record(ctx, int64(stats.Idle), metric.WithAttributes(o.attributes...))

	o.mu.Lock()
	waitCountDelta := stats.WaitCount - o.previousWaits
	waitDurationDelta := stats.WaitDuration - o.previousWaitTime
	o.previousWaits = stats.WaitCount
	o.previousWaitTime = stats.WaitDuration
	o.mu.Unlock()

	if waitCountDelta > 0 {
		o.waitCount.Add(ctx, waitCountDelta, metric.WithAttributes(o.attributes...))
	}
	if waitDurationDelta > 0 {
		o.waitDuration.Add(ctx, waitDurationDelta.Seconds(), metric.WithAttributes(o.attributes...))
	}
}

// NATSConsumerSnapshot is the application-level view of a NATS consumer. Map
// client-specific consumer information into this type in the application; no
// NATS package is imported by goslogx or this reference.
type NATSConsumerSnapshot struct {
	PendingMessages     int64
	AckPending          int64
	RedeliveredMessages int64
	Lag                 int64
}

// NATSMetrics records low-cardinality NATS consumer and operation metrics.
type NATSMetrics struct {
	consumerAttributes []attribute.KeyValue
	publishAttributes  []attribute.KeyValue

	pendingMessages     metric.Int64Gauge
	ackPending          metric.Int64Gauge
	redeliveredMessages metric.Int64Gauge
	lag                 metric.Int64Gauge
	publishMessages     metric.Int64Counter
	consumeMessages     metric.Int64Counter
	operationDuration   metric.Float64Histogram
}

// NewNATSMetrics creates NATS instruments. destinationName and consumerName
// must be stable names such as "payments" and "payment-worker"; never use a
// message ID as a metric attribute.
func NewNATSMetrics(logger *goslogx.Logger, instrumentationScope, destinationName, consumerName string) (*NATSMetrics, error) {
	if logger == nil {
		return nil, errors.New("otel nats metrics: logger is required")
	}
	if instrumentationScope == "" {
		return nil, errors.New("otel nats metrics: instrumentation scope is required")
	}
	if destinationName == "" || consumerName == "" {
		return nil, errors.New("otel nats metrics: destination and consumer names are required")
	}

	meter := logger.MeterProvider().Meter(instrumentationScope)
	pendingMessages, err := meter.Int64Gauge("messaging.consumer.pending_messages", metric.WithUnit("{message}"))
	if err != nil {
		return nil, err
	}
	ackPending, err := meter.Int64Gauge("messaging.consumer.ack_pending", metric.WithUnit("{message}"))
	if err != nil {
		return nil, err
	}
	redeliveredMessages, err := meter.Int64Gauge("messaging.consumer.redelivered_messages", metric.WithUnit("{message}"))
	if err != nil {
		return nil, err
	}
	lag, err := meter.Int64Gauge("messaging.consumer.lag", metric.WithUnit("{message}"))
	if err != nil {
		return nil, err
	}
	publishMessages, err := meter.Int64Counter("messaging.publish.messages", metric.WithUnit("{message}"))
	if err != nil {
		return nil, err
	}
	consumeMessages, err := meter.Int64Counter("messaging.consume.messages", metric.WithUnit("{message}"))
	if err != nil {
		return nil, err
	}
	operationDuration, err := meter.Float64Histogram("messaging.operation.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}

	baseAttributes := []attribute.KeyValue{
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.destination.name", destinationName),
	}
	consumerAttributes := append([]attribute.KeyValue{}, baseAttributes...)
	consumerAttributes = append(consumerAttributes, attribute.String("messaging.consumer.name", consumerName))

	return &NATSMetrics{
		consumerAttributes:  consumerAttributes,
		publishAttributes:   baseAttributes,
		pendingMessages:     pendingMessages,
		ackPending:          ackPending,
		redeliveredMessages: redeliveredMessages,
		lag:                 lag,
		publishMessages:     publishMessages,
		consumeMessages:     consumeMessages,
		operationDuration:   operationDuration,
	}, nil
}

// ObserveConsumer records the current consumer snapshot. The caller maps its
// NATS client response into NATSConsumerSnapshot before calling this method.
func (m *NATSMetrics) ObserveConsumer(ctx context.Context, snapshot NATSConsumerSnapshot) {
	if m == nil {
		return
	}
	m.pendingMessages.Record(ctx, snapshot.PendingMessages, metric.WithAttributes(m.consumerAttributes...))
	m.ackPending.Record(ctx, snapshot.AckPending, metric.WithAttributes(m.consumerAttributes...))
	m.redeliveredMessages.Record(ctx, snapshot.RedeliveredMessages, metric.WithAttributes(m.consumerAttributes...))
	m.lag.Record(ctx, snapshot.Lag, metric.WithAttributes(m.consumerAttributes...))
}

// RecordPublish records one completed publish operation.
func (m *NATSMetrics) RecordPublish(ctx context.Context, duration time.Duration) {
	if m == nil {
		return
	}
	m.publishMessages.Add(ctx, 1, metric.WithAttributes(m.publishAttributes...))
	m.operationDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(m.publishAttributes...))
}

// RecordConsume records one completed consumer operation.
func (m *NATSMetrics) RecordConsume(ctx context.Context, duration time.Duration) {
	if m == nil {
		return
	}
	m.consumeMessages.Add(ctx, 1, metric.WithAttributes(m.consumerAttributes...))
	m.operationDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(m.consumerAttributes...))
}
