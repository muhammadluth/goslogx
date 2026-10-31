package bun

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/muhammadluth/goslogx/internal/integration/privacy"
	"reflect"
	"runtime"
	"sync"
	"time"
	"weak"

	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	upbun "github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
	"github.com/uptrace/bun/extra/bunotel"
	upotelsql "github.com/uptrace/opentelemetry-go-extra/otelsql"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

const instrumentationScope = "github.com/muhammadluth/goslogx/plugins/database/bun"

type Option func(*config)

type config struct {
	attributes    []attribute.KeyValue
	statement     bool
	policy        telemetry.Config
	caller        bool
	legacyMetrics bool
	poolName      string
	formatter     func(*upbun.QueryEvent) string
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

func WithAttributes(attributes ...attribute.KeyValue) Option {
	copied := append([]attribute.KeyValue(nil), attributes...)
	return func(c *config) { c.attributes = append(c.attributes, privacy.Attributes(copied)...) }
}

// WithQueryStatement enables capture requiring WithStatementSanitizer.
// Use WithRawStatement for deliberately unsanitized diagnostic capture.
func WithQueryStatement(enabled bool) Option {
	return func(c *config) { c.statement = enabled }
}

func Instrument(db *upbun.DB, provider goslogx.TelemetryProvider, opts ...Option) (err error) {
	if db == nil {
		return errors.New("bun instrumentation: nil database")
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
	c := config{poolName: "bun"}
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
		return errors.New("bun instrumentation: database already instrumented")
	}
	defer func() {
		if err != nil {
			registered.Delete(key)
		}
	}()
	runtime.AddCleanup(db, func(key weak.Pointer[upbun.DB]) { registered.Delete(key) }, key)
	meter := mp.Meter(instrumentationScope)
	metricAttrs := append(append([]attribute.KeyValue(nil), c.attributes...), c.policy.MetricAttributes...)
	var duration metric.Float64Histogram
	if c.legacyMetrics {
		var err error
		duration, err = meter.Float64Histogram("db.client.operation.duration", metric.WithUnit("s"))
		if err != nil {
			return err
		}
		poolAttrs := append(append([]attribute.KeyValue(nil), metricAttrs...), attribute.String("db.client.connection.pool.name", c.poolName))
		if err := observePool(meter, db, poolAttrs); err != nil {
			return err
		}
	}
	var initErr error
	formatter := c.formatter
	if formatter == nil {
		formatter = func(event *upbun.QueryEvent) string { return safeOperation(event.Operation()) }
	}
	hook := queryHook{bunotel.NewQueryHook(
		bunotel.WithTracerProvider(privateTracerProvider{TracerProvider: tp, config: c}),
		bunotel.WithMeterProvider(scopedMeterProvider{MeterProvider: mp, attributes: c.policy.MetricAttributes, initErr: &initErr}),
		bunotel.WithAttributes(c.attributes...),
		bunotel.WithFormattedQueries(c.policy.RawStatement || c.policy.StatementSanitizer != nil),
		bunotel.WithSpanNameQueryGen(formatter),
	)}
	if initErr != nil {
		return initErr
	}
	if err := upstreamPool(db, scopedMeterProvider{MeterProvider: mp, attributes: c.policy.MetricAttributes}, metricAttrs); err != nil {
		return err
	}
	db.AddQueryHook(hook)
	// bunotel owns tracing and its upstream compatibility metrics. Keep the
	// stable goslogx semconv histogram as a metrics-only hook for existing users.
	if c.legacyMetrics {
		db.AddQueryHook(&metricsHook{duration: duration, attributes: metricAttrs})
	}
	return nil
}

type metricsHook struct {
	duration   metric.Float64Histogram
	attributes []attribute.KeyValue
}

func (*metricsHook) Init(*upbun.DB) {}

func (h *metricsHook) BeforeQuery(ctx context.Context, _ *upbun.QueryEvent) context.Context {
	return ctx
}

func (h *metricsHook) AfterQuery(ctx context.Context, event *upbun.QueryEvent) {
	attrs := h.spanAttributes(event, safeOperation(event.Operation()))
	if event.Err != nil && !errors.Is(event.Err, sql.ErrNoRows) && !errors.Is(event.Err, sql.ErrTxDone) {
		attrs = append(attrs, attribute.String("error.type", "database_error"))
	}
	if h.duration != nil {
		h.duration.Record(ctx, time.Since(event.StartTime).Seconds(), metric.WithAttributes(attrs...))
	}
}

func (h *metricsHook) spanAttributes(event *upbun.QueryEvent, operation string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(h.attributes)+3)
	attrs = append(attrs, h.attributes...)
	attrs = append(attrs, attribute.String("db.operation.name", safeOperation(operation)))
	if event.DB != nil && event.DB.Dialect() != nil {
		switch event.DB.Dialect().Name() {
		case dialect.PG:
			attrs = append(attrs, attribute.String("db.system.name", "postgresql"))
		case dialect.MySQL:
			attrs = append(attrs, attribute.String("db.system.name", "mysql"))
		case dialect.SQLite:
			attrs = append(attrs, attribute.String("db.system.name", "sqlite"))
		case dialect.MSSQL:
			attrs = append(attrs, attribute.String("db.system.name", "microsoft.sql_server"))
		}
	}
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

// queryHook delegates query telemetry, but intentionally replaces upstream Init:
// bunotel 1.2.18 registers pool metrics on the global meter there.
type queryHook struct{ *bunotel.QueryHook }

func (queryHook) Init(*upbun.DB) {}

// WithCompatibilityMetrics also emits the pre-migration goslogx DB instruments.
// Default metrics use the native bunotel/otelsql names and units only.
func WithCompatibilityMetrics(enabled bool) Option {
	return func(c *config) { c.legacyMetrics = enabled }
}
func upstreamPool(db *upbun.DB, mp metric.MeterProvider, attrs []attribute.KeyValue) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("bun: upstream pool metric registration failed: %v", r)
		}
	}()
	upotelsql.ReportDBStatsMetrics(db.DB, upotelsql.WithMeterProvider(mp), upotelsql.WithAttributes(attrs...))
	return nil
}

var registered sync.Map

// WithSpanNameFormatter maps bunotel's query-event formatter. Return operation
// labels rather than raw SQL; the formatter controls spans, never metric labels.
func WithSpanNameFormatter(f func(*upbun.QueryEvent) string) Option {
	return func(c *config) { c.formatter = f }
}
