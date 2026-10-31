package pg

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"weak"

	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/privacy"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

const instrumentationScope = "github.com/muhammadluth/goslogx/plugins/database/pg"

type Option func(*config)

type config struct {
	attributes []attribute.KeyValue
	statement  bool
	policy     telemetry.Config
	caller     bool
	poolName   string
}

// WithPoolName distinguishes application-owned pools using a fixed name.
func WithPoolName(name string) Option {
	return func(c *config) {
		if name != "" && len(name) <= 256 {
			c.poolName = name
		}
	}
}

func WithDBName(name string) Option {
	return func(c *config) {
		if name != "" && len(name) <= 256 {
			c.attributes = append(c.attributes, attribute.String("db.namespace", name))
		}
	}
}

// WithQueryStatement enables capture requiring WithStatementSanitizer.
// Use WithRawStatement for deliberately unsanitized diagnostic capture.
func WithQueryStatement(enabled bool) Option {
	return func(c *config) { c.statement = enabled }
}

func Instrument(db *pg.DB, provider goslogx.TelemetryProvider, opts ...Option) (err error) {
	if db == nil {
		return errors.New("pg instrumentation: nil database")
	}
	var tp trace.TracerProvider = tracenoop.NewTracerProvider()
	var mp metric.MeterProvider = metricnoop.NewMeterProvider()
	if !isNil(provider) {
		if p := provider.TracerProvider(); !isNil(p) {
			tp = p
		}
		if p := provider.MeterProvider(); !isNil(p) {
			mp = p
		}
	}
	c := config{poolName: "pg"}
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	c.policy.StatementCapture = c.statement || c.policy.StatementCapture
	if c.policy.StatementCapture && c.policy.StatementSanitizer == nil && !c.policy.RawStatement {
		return errors.New("database instrumentation: statement capture requires a sanitizer")
	}
	if c.policy.TracingDisabled {
		tp = tracenoop.NewTracerProvider()
	}
	if c.policy.MetricsDisabled {
		mp = metricnoop.NewMeterProvider()
	}
	key := weak.Make(db)
	if _, loaded := registered.LoadOrStore(key, struct{}{}); loaded {
		return errors.New("pg instrumentation: database already instrumented")
	}
	defer func() {
		if err != nil {
			registered.Delete(key)
		}
	}()
	runtime.AddCleanup(db, func(key weak.Pointer[pg.DB]) { registered.Delete(key) }, key)
	meter := mp.Meter(instrumentationScope)
	duration, err := meter.Float64Histogram("db.client.operation.duration", metric.WithUnit("s"), metric.WithDescription("Database operation duration."), metric.WithExplicitBucketBoundaries(.001, .005, .01, .05, .1, .5, 1, 5, 10))
	if err != nil {
		return err
	}
	poolAttributes := append(append(append([]attribute.KeyValue(nil), c.attributes...), c.policy.MetricAttributes...), attribute.String("db.client.connection.pool.name", c.poolName))
	if err := observePool(meter, db, poolAttributes); err != nil {
		return err
	}
	db.AddQueryHook(&queryHook{tracer: tp.Tracer(instrumentationScope), duration: duration, attributes: c.attributes, config: c})
	return nil
}

type queryHook struct {
	config     config
	tracer     trace.Tracer
	duration   metric.Float64Histogram
	attributes []attribute.KeyValue
}

func (*queryHook) Init(*pg.DB) {}

func (h *queryHook) BeforeQuery(ctx context.Context, event *pg.QueryEvent) (context.Context, error) {
	operation := queryOperation(event)
	name := safeOperation(string(operation))
	if name == "" {
		name = "QUERY"
	}
	attrs := append(h.spanAttributes(event, operation), h.config.policy.TraceAttributes...)
	if db, ok := event.DB.(*pg.DB); ok && db != nil {
		opts := db.Options()
		if host, port, err := net.SplitHostPort(opts.Addr); err == nil {
			attrs = append(attrs, attribute.String("server.address", host))
			if port, err := strconv.Atoi(port); err == nil {
				attrs = append(attrs, attribute.Int("server.port", port))
			}
		}
	}
	if h.config.policy.StatementCapture {
		if query, err := event.FormattedQuery(); err == nil {
			attrs = append(attrs, attribute.String("db.statement", h.config.policy.QueryText(string(query))))
		}
	}
	if h.config.caller {
		if pc, file, line, ok := runtime.Caller(2); ok {
			attrs = append(attrs, attribute.String("code.filepath", file), attribute.Int("code.lineno", line))
			if fn := runtime.FuncForPC(pc); fn != nil {
				attrs = append(attrs, attribute.String("code.function", fn.Name()))
			}
		}
	}
	ctx, _ = h.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
	return ctx, nil
}

func (h *queryHook) AfterQuery(ctx context.Context, event *pg.QueryEvent) error {
	span := trace.SpanFromContext(ctx)
	operation := queryOperation(event)
	attrs := h.spanAttributes(event, operation)
	if event.Err != nil && !errors.Is(event.Err, sql.ErrNoRows) && !errors.Is(event.Err, sql.ErrTxDone) && !errors.Is(event.Err, pg.ErrNoRows) && !errors.Is(event.Err, pg.ErrMultiRows) {
		attrs = append(attrs, attribute.String("error.type", privacy.ErrorType(event.Err)))
		privacy.RecordError(span, event.Err)
		span.SetStatus(codes.Error, privacy.StatusDescription(codes.Error, ""))
	} else if event.Err == nil && !isNil(event.Result) {
		rows := event.Result.RowsAffected()
		if rows < 0 {
			rows = event.Result.RowsReturned()
		}
		if rows >= 0 {
			span.SetAttributes(attribute.Int("db.rows_affected", rows))
		}
	}
	if h.duration != nil {
		attrs = append(attrs, h.config.policy.MetricAttributes...)
		h.duration.Record(ctx, time.Since(event.StartTime).Seconds(), metric.WithAttributes(attrs...))
	}
	span.End()
	return nil
}

func queryOperation(event *pg.QueryEvent) orm.QueryOp {
	if query, ok := event.Query.(interface{ Operation() orm.QueryOp }); ok {
		return query.Operation()
	}
	query, err := event.UnformattedQuery()
	if err != nil {
		return orm.QueryOp("QUERY")
	}
	parts := strings.Fields(string(query))
	if len(parts) == 0 {
		return orm.QueryOp("QUERY")
	}
	return orm.QueryOp(safeOperation(parts[0]))
}

func (h *queryHook) spanAttributes(event *pg.QueryEvent, operation orm.QueryOp) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(h.attributes)+3)
	attrs = append(attrs, h.attributes...)
	attrs = append(attrs, attribute.String("db.operation.name", safeOperation(string(operation))))
	attrs = append(attrs, attribute.String("db.system.name", "postgresql"))
	return attrs
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func WithTracing(enabled bool) Option { return func(c *config) { c.policy.TracingDisabled = !enabled } }
func WithMetrics(enabled bool) Option { return func(c *config) { c.policy.MetricsDisabled = !enabled } }
func WithStatementCapture(enabled bool) Option {
	return func(c *config) { c.policy.StatementCapture = enabled }
}
func WithStatementSanitizer(f func(string) string) Option {
	return func(c *config) { c.policy.StatementSanitizer = f }
}
func WithRawStatement(enabled bool) Option {
	return func(c *config) { c.policy.RawStatement = enabled; c.policy.StatementCapture = enabled }
}
func WithCallerEnabled(enabled bool) Option { return func(c *config) { c.caller = enabled } }
func WithTraceAttributes(attrs ...attribute.KeyValue) Option {
	return func(c *config) { telemetry.WithTraceAttributes(attrs...)(&c.policy) }
}
func WithMetricAttributes(attrs ...attribute.KeyValue) Option {
	return func(c *config) { telemetry.WithMetricAttributes(attrs...)(&c.policy) }
}

var registered sync.Map
