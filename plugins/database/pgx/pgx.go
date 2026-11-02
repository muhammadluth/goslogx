// Package pgx instruments pgx v5 connection and pool configurations.
package pgx

import (
	"context"
	"errors"
	"fmt"
	"github.com/exaring/otelpgx"

	native "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/database/pgx"

// Upstream owns telemetry. The adapter only fills its root-span gap:
// otelpgx 0.13.0 skips starting a span without an already recording parent.
type tracer struct {
	*otelpgx.Tracer
	root   trace.Tracer
	config config
}

func newTracer(p goslogx.TelemetryProvider, opts []Option) (*tracer, error) {
	cfg := config{}
	telemetry.ApplyOptions(&cfg.Config, &cfg, opts...)
	if cfg.StatementCapture && cfg.StatementSanitizer == nil && !cfg.RawStatement {
		return nil, errors.New("pgx: statement capture requires a sanitizer")
	}
	tp, mp, _ := telemetry.Resolve(p, cfg.Config)
	tp = telemetry.ConfiguredDBProvider(tp, ScopeName, cfg.Config, telemetry.DBPolicy{ExtraAttributes: cfg.parameterAttributes})
	attrs := append([]attribute.KeyValue(nil), cfg.TraceAttributes...)
	metricAttrs := append([]attribute.KeyValue(nil), cfg.MetricAttributes...)
	if cfg.Namespace != "" {
		attrs = append(attrs, attribute.String("db.namespace", cfg.Namespace))
		metricAttrs = append(metricAttrs, attribute.String("db.namespace", cfg.Namespace))
	}
	var initErr error
	nativeOptions := []otelpgx.Option{otelpgx.WithTracerProvider(tp), otelpgx.WithMeterProvider(scopedMeter{MeterProvider: mp, initErr: &initErr}), otelpgx.WithTracerAttributes(attrs...), otelpgx.WithMeterAttributes(metricAttrs...), otelpgx.WithSpanNameCtxFunc(cfg.operation), otelpgx.WithMetricOperationName(operation)}
	if cfg.parameters != nil || cfg.rawParameters {
		nativeOptions = append(nativeOptions, otelpgx.WithIncludeQueryParameters(cfg.parameterFilter))
	}
	if cfg.acquireDisabled {
		nativeOptions = append(nativeOptions, otelpgx.WithDisableAcquireTracer())
	}
	if cfg.connectionDetailsDisabled {
		nativeOptions = append(nativeOptions, otelpgx.WithDisableConnectionDetailsInAttributes())
	}
	if cfg.prefix {
		nativeOptions = append(nativeOptions, otelpgx.WithQuerySpanNamePrefix())
	}
	native := otelpgx.NewTracer(nativeOptions...)
	if initErr != nil {
		return nil, initErr
	}
	return &tracer{Tracer: native, root: tp.Tracer(ScopeName, trace.WithInstrumentationVersion("0.13.0")), config: cfg}, nil
}
func operation(ctx context.Context, sql string) string {
	return telemetry.DBOperation(otelpgx.SQLOperationName(ctx, sql))
}
func op(sql string) string { return operation(context.Background(), sql) }
func (t *tracer) rootContext(parent, ctx context.Context, name, sql string, cfg *native.ConnConfig, attrs ...attribute.KeyValue) context.Context {
	if trace.SpanFromContext(parent).IsRecording() {
		return ctx
	}
	attrs = append(attrs, attribute.String("db.system.name", "postgresql"))
	if sql != "" {
		attrs = append(attrs, attribute.String("db.operation.name", t.config.operation(ctx, sql)), attribute.String("db.query.text", sql))
	}
	if cfg != nil && !t.config.connectionDetailsDisabled {
		attrs = append(attrs, attribute.String("server.address", cfg.Host), attribute.Int("server.port", int(cfg.Port)), attribute.String("db.namespace", cfg.Database))
	}
	ctx, _ = t.root.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
	return ctx
}
func connConfig(conn *native.Conn) *native.ConnConfig {
	if conn == nil {
		return nil
	}
	return conn.Config()
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
func (t *tracer) TraceQueryStart(ctx context.Context, conn *native.Conn, data native.TraceQueryStartData) context.Context {
	var attrs []attribute.KeyValue
	if !trace.SpanFromContext(ctx).IsRecording() && (t.config.parameters != nil || t.config.rawParameters) && (t.config.parameterFilter == nil || t.config.parameterFilter(data.SQL)) {
		values := make([]string, len(data.Args))
		for i, arg := range data.Args {
			values[i] = fmt.Sprintf("%+v", arg)
		}
		attrs = append(attrs, attribute.StringSlice("pgx.query.parameters", values))
	}
	return t.rootContext(ctx, t.Tracer.TraceQueryStart(ctx, conn, data), t.config.name(ctx, data.SQL, "query "), data.SQL, connConfig(conn), attrs...)
}
func (t *tracer) TraceBatchStart(ctx context.Context, conn *native.Conn, data native.TraceBatchStartData) context.Context {
	size := 0
	if data.Batch != nil {
		size = data.Batch.Len()
	}
	return t.rootContext(ctx, t.Tracer.TraceBatchStart(ctx, conn, data), "batch start", "", connConfig(conn), attribute.Int("db.operation.batch.size", size))
}
func (t *tracer) TraceCopyFromStart(ctx context.Context, conn *native.Conn, data native.TraceCopyFromStartData) context.Context {
	return t.rootContext(ctx, t.Tracer.TraceCopyFromStart(ctx, conn, data), "copy_from "+data.TableName.Sanitize(), "", connConfig(conn), attribute.String("db.collection.name", data.TableName.Sanitize()))
}
func (t *tracer) TraceConnectStart(ctx context.Context, data native.TraceConnectStartData) context.Context {
	return t.rootContext(ctx, t.Tracer.TraceConnectStart(ctx, data), "connect", "", data.ConnConfig)
}
func (t *tracer) TracePrepareStart(ctx context.Context, conn *native.Conn, data native.TracePrepareStartData) context.Context {
	return t.rootContext(ctx, t.Tracer.TracePrepareStart(ctx, conn, data), t.config.name(ctx, data.SQL, "prepare "), data.SQL, connConfig(conn), attribute.String("pgx.prepare_stmt.name", data.Name))
}
func (t *tracer) TraceAcquireStart(ctx context.Context, pool *pgxpool.Pool, data pgxpool.TraceAcquireStartData) context.Context {
	if t.config.acquireDisabled {
		return ctx
	}
	var cfg *native.ConnConfig
	if pool != nil {
		cfg = pool.Config().ConnConfig
	}
	return t.rootContext(ctx, t.Tracer.TraceAcquireStart(ctx, pool, data), "pool.acquire", "", cfg)
}

// ObservePool adds pool observations after pool construction. Provider and pool lifetimes should match.
func ObservePool(pool *pgxpool.Pool, p goslogx.TelemetryProvider, opts ...Option) error {
	if pool == nil {
		return errors.New("pgx instrumentation: nil pool")
	}
	cfg := config{}
	telemetry.ApplyOptions(&cfg.Config, &cfg, opts...)
	_, mp, _ := telemetry.Resolve(p, cfg.Config)
	attrs := append([]attribute.KeyValue{attribute.String("db.client.connection.pool.name", "pgx")}, cfg.MetricAttributes...)
	if cfg.Namespace != "" {
		attrs = append(attrs, attribute.String("db.namespace", cfg.Namespace))
	}
	return otelpgx.RecordStats(pool, otelpgx.WithStatsMeterProvider(scopedMeter{MeterProvider: mp}), otelpgx.WithStatsAttributes(attrs...), otelpgx.WithMinimumReadDBStatsInterval(cfg.poolStatsInterval))
}

type scopedMeter struct {
	metric.MeterProvider
	initErr *error
}

func (p scopedMeter) Meter(_ string, opts ...metric.MeterOption) metric.Meter {
	return initializingMeter{Meter: p.MeterProvider.Meter(ScopeName, opts...), initErr: p.initErr}
}

var _ native.QueryTracer = (*tracer)(nil)
var _ native.BatchTracer = (*tracer)(nil)
var _ native.CopyFromTracer = (*tracer)(nil)
var _ native.ConnectTracer = (*tracer)(nil)

var _ native.PrepareTracer = (*tracer)(nil)
var _ pgxpool.AcquireTracer = (*tracer)(nil)

// Upstream reports instrument failures through otel.Handle; also return them to
// the caller before installing a tracer that could hold an invalid instrument.
type initializingMeter struct {
	metric.Meter
	initErr *error
}

func (m initializingMeter) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	h, err := m.Meter.Float64Histogram(name, opts...)
	if err != nil && m.initErr != nil {
		*m.initErr = errors.Join(*m.initErr, err)
	}
	return h, err
}
func (m initializingMeter) Int64Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	c, err := m.Meter.Int64Counter(name, opts...)
	if err != nil && m.initErr != nil {
		*m.initErr = errors.Join(*m.initErr, err)
	}
	return c, err
}
