// Package cassandra instruments Apache gocql observers and explicit iterator lifetimes.
package cassandra

import (
	"context"
	"errors"
	native "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/privacy"
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
var WithRawStatement = telemetry.WithRawStatement
var WithStatementCapture = telemetry.WithStatementCapture
var WithStatementSanitizer = telemetry.WithStatementSanitizer
var WithTraceAttributes = telemetry.WithTraceAttributes
var WithMetricAttributes = telemetry.WithMetricAttributes
var WithPropagator = telemetry.WithPropagator
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
func (o *Observer) record(ctx context.Context, op, q string, start, end time.Time, err error, metadata ...attribute.KeyValue) {
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
	attrs = append(attrs, o.t.Config.TraceAttributes...)
	attrs = append(attrs, metadata...)
	ctx, span := o.t.Tracer.Start(ctx, op, trace.WithSpanKind(trace.SpanKindClient), trace.WithTimestamp(start), trace.WithAttributes(attrs...))
	o.t.Statement(ctx, q)
	if err != nil {
		privacy.RecordError(span, err)
		span.SetStatus(codes.Error, privacy.StatusDescription(codes.Error, ""))
	}
	span.End(trace.WithTimestamp(end))
	o.t.RecordDatabase(ctx, op, end.Sub(start), err)
}
func (o *Observer) ObserveQuery(ctx context.Context, q native.ObservedQuery) {
	op := "QUERY"
	if f := strings.Fields(q.Statement); len(f) > 0 {
		op = telemetry.DBOperation(f[0])
	}
	attrs := hostAttributes(q.Host)
	if q.Err == nil && q.Rows >= 0 {
		attrs = append(attrs, attribute.Int("db.response.returned_rows", q.Rows))
	}
	o.record(ctx, op, q.Statement, q.Start, q.End, q.Err, attrs...)
}
func (o *Observer) ObserveBatch(ctx context.Context, b native.ObservedBatch) {
	attrs := append(hostAttributes(b.Host), attribute.Int("db.operation.batch.size", len(b.Statements)))
	o.record(ctx, "BATCH", "", b.Start, b.End, b.Err, attrs...)
}

func hostAttributes(host *native.HostInfo) []attribute.KeyValue {
	if host == nil {
		return nil
	}
	return []attribute.KeyValue{attribute.String("server.address", host.ConnectAddress().String()), attribute.Int("server.port", host.Port())}
}

// Iter deliberately exposes the native iterator. Always Close it inside fn so
// late protocol/scan errors are returned and included in the lifecycle span.
func (o *Observer) Iter(ctx context.Context, fn func(context.Context) error) error {
	return o.t.Run(ctx, "NEXT", trace.SpanKindInternal, fn)
}

var _ native.QueryObserver = (*Observer)(nil)
var _ native.BatchObserver = (*Observer)(nil)
