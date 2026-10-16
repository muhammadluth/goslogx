// Package pgx instruments pgx v5 connection and pool configurations.
package pgx

import (
	"context"
	"errors"
	"strings"

	native "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/database/pgx"

type Option = telemetry.Option

var WithDBName = telemetry.WithDBName
var WithStatementCapture = telemetry.WithStatementCapture
var WithStatementSanitizer = telemetry.WithStatementSanitizer
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

type tracer struct{ t *telemetry.Instrumentation }
type queryKey struct{}
type batchKey struct{}
type copyKey struct{}
type connectKey struct{}

func newTracer(p goslogx.TelemetryProvider, opts []Option) (*tracer, error) {
	t, err := telemetry.New(p, ScopeName, "postgresql", false, opts...)
	if err != nil {
		return nil, err
	}
	return &tracer{t}, nil
}

// InstrumentConfig configures a pool before construction. Existing tracers are never overwritten.
func InstrumentConfig(config *pgxpool.Config, p goslogx.TelemetryProvider, opts ...Option) error {
	if config == nil || config.ConnConfig == nil {
		return errors.New("pgx instrumentation: nil pool config")
	}
	return InstrumentConnConfig(config.ConnConfig, p, opts...)
}
func InstrumentConnConfig(config *native.ConnConfig, p goslogx.TelemetryProvider, opts ...Option) error {
	if config == nil {
		return errors.New("pgx instrumentation: nil connection config")
	}
	if !telemetry.Nil(config.Tracer) {
		return errors.New("pgx instrumentation: tracer already configured; choose one instrumentation layer")
	}
	t, err := newTracer(p, opts)
	if err != nil {
		return err
	}
	config.Tracer = t
	return nil
}
func op(query string) string {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return "QUERY"
	}
	return telemetry.DBOperation(fields[0])
}
func (t *tracer) TraceQueryStart(ctx context.Context, _ *native.Conn, data native.TraceQueryStartData) context.Context {
	ctx, finish := t.t.Begin(ctx, op(data.SQL), "", trace.SpanKindClient)
	t.t.Statement(ctx, data.SQL)
	return context.WithValue(ctx, queryKey{}, finish)
}
func (t *tracer) TraceQueryEnd(ctx context.Context, _ *native.Conn, data native.TraceQueryEndData) {
	if finish, ok := ctx.Value(queryKey{}).(func(error)); ok {
		err := data.Err
		if errors.Is(err, native.ErrNoRows) {
			err = nil
		}
		finish(err)
	}
}
func (t *tracer) TraceBatchStart(ctx context.Context, _ *native.Conn, _ native.TraceBatchStartData) context.Context {
	ctx, finish := t.t.Begin(ctx, "BATCH", "", trace.SpanKindClient)
	return context.WithValue(ctx, batchKey{}, finish)
}
func (t *tracer) TraceBatchQuery(ctx context.Context, _ *native.Conn, data native.TraceBatchQueryData) {
	ctx, finish := t.t.Begin(ctx, op(data.SQL), "", trace.SpanKindClient)
	t.t.Statement(ctx, data.SQL)
	finish(data.Err)
}
func (t *tracer) TraceBatchEnd(ctx context.Context, _ *native.Conn, data native.TraceBatchEndData) {
	if finish, ok := ctx.Value(batchKey{}).(func(error)); ok {
		finish(data.Err)
	}
}
func (t *tracer) TraceCopyFromStart(ctx context.Context, _ *native.Conn, _ native.TraceCopyFromStartData) context.Context {
	ctx, finish := t.t.Begin(ctx, "COPY", "", trace.SpanKindClient)
	return context.WithValue(ctx, copyKey{}, finish)
}
func (t *tracer) TraceCopyFromEnd(ctx context.Context, _ *native.Conn, data native.TraceCopyFromEndData) {
	if finish, ok := ctx.Value(copyKey{}).(func(error)); ok {
		finish(data.Err)
	}
}
func (t *tracer) TraceConnectStart(ctx context.Context, _ native.TraceConnectStartData) context.Context {
	ctx, finish := t.t.Begin(ctx, "CONNECT", "", trace.SpanKindClient)
	return context.WithValue(ctx, connectKey{}, finish)
}
func (t *tracer) TraceConnectEnd(ctx context.Context, data native.TraceConnectEndData) {
	if finish, ok := ctx.Value(connectKey{}).(func(error)); ok {
		finish(data.Err)
	}
}

// ObservePool adds pool observations after pool construction. Provider and pool lifetimes should match.
func ObservePool(pool *pgxpool.Pool, p goslogx.TelemetryProvider, opts ...Option) error {
	if pool == nil {
		return errors.New("pgx instrumentation: nil pool")
	}
	cfg := telemetry.Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	_, mp, _ := telemetry.Resolve(p, cfg)
	meter := mp.Meter(ScopeName)
	attrs := []attribute.KeyValue{attribute.String("db.system.name", "postgresql")}
	if cfg.Namespace != "" {
		attrs = append(attrs, attribute.String("db.namespace", cfg.Namespace))
	}
	_, err := meter.Int64ObservableUpDownCounter("db.client.connection.count", metric.WithUnit("{connection}"), metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
		s := pool.Stat()
		used := append(append([]attribute.KeyValue(nil), attrs...), attribute.String("db.client.connection.state", "used"))
		idle := append(append([]attribute.KeyValue(nil), attrs...), attribute.String("db.client.connection.state", "idle"))
		o.Observe(int64(s.AcquiredConns()), metric.WithAttributes(used...))
		o.Observe(int64(s.IdleConns()), metric.WithAttributes(idle...))
		return nil
	}))
	if err != nil {
		return err
	}
	_, err = meter.Int64ObservableUpDownCounter("db.client.connection.max", metric.WithUnit("{connection}"), metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
		o.Observe(int64(pool.Stat().MaxConns()), metric.WithAttributes(attrs...))
		return nil
	}))
	return err
}

var _ native.QueryTracer = (*tracer)(nil)
var _ native.BatchTracer = (*tracer)(nil)
var _ native.CopyFromTracer = (*tracer)(nil)
var _ native.ConnectTracer = (*tracer)(nil)
