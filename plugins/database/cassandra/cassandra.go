// Package cassandra instruments Apache gocql observers and explicit iterator lifetimes.
package cassandra

import (
	"context"
	"errors"
	native "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"strings"
	"time"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/database/cassandra"

type Option = telemetry.Option

var WithDBName = telemetry.WithDBName
var WithStatementCapture = telemetry.WithStatementCapture
var WithStatementSanitizer = telemetry.WithStatementSanitizer
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

type Observer struct{ t *telemetry.Instrumentation }

func New(p goslogx.TelemetryProvider, opts ...Option) (*Observer, error) {
	t, err := telemetry.New(p, ScopeName, "cassandra", false, opts...)
	if err != nil {
		return nil, err
	}
	return &Observer{t}, nil
}
func InstrumentConfig(cfg *native.ClusterConfig, p goslogx.TelemetryProvider, opts ...Option) error {
	if cfg == nil {
		return errors.New("cassandra: nil config")
	}
	if !telemetry.Nil(cfg.QueryObserver) || !telemetry.Nil(cfg.BatchObserver) {
		return errors.New("cassandra: observers already configured; choose one instrumentation layer")
	}
	o, err := New(p, opts...)
	if err != nil {
		return err
	}
	cfg.QueryObserver = o
	cfg.BatchObserver = o
	return nil
}
func (o *Observer) record(ctx context.Context, op, q string, start, end time.Time, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if start.IsZero() {
		start = time.Now()
	}
	if end.Before(start) {
		end = start
	}
	attrs := []attribute.KeyValue{attribute.String("db.system.name", "cassandra"), attribute.String("db.operation.name", op)}
	if o.t.Config.Namespace != "" {
		attrs = append(attrs, attribute.String("db.namespace", o.t.Config.Namespace))
	}
	ctx, span := o.t.Tracer.Start(ctx, op, trace.WithSpanKind(trace.SpanKindClient), trace.WithTimestamp(start), trace.WithAttributes(attrs...))
	o.t.Statement(ctx, q)
	if err != nil {
		span.SetAttributes(attribute.String("error.type", "database_error"))
		span.SetStatus(codes.Error, "")
	}
	span.End(trace.WithTimestamp(end))
	o.t.RecordDatabase(ctx, op, end.Sub(start), err)
}
func (o *Observer) ObserveQuery(ctx context.Context, q native.ObservedQuery) {
	op := "QUERY"
	if f := strings.Fields(q.Statement); len(f) > 0 {
		op = telemetry.DBOperation(f[0])
	}
	o.record(ctx, op, q.Statement, q.Start, q.End, q.Err)
}
func (o *Observer) ObserveBatch(ctx context.Context, b native.ObservedBatch) {
	o.record(ctx, "BATCH", "", b.Start, b.End, b.Err)
}

// Iter deliberately exposes the native iterator. Always Close it inside fn so
// late protocol/scan errors are returned and included in the lifecycle span.
func (o *Observer) Iter(ctx context.Context, fn func(context.Context) error) error {
	return o.t.Run(ctx, "NEXT", trace.SpanKindInternal, fn)
}

var _ native.QueryObserver = (*Observer)(nil)
var _ native.BatchObserver = (*Observer)(nil)
