package bun

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"time"

	"github.com/muhammadluth/goslogx"
	upbun "github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

const instrumentationScope = "github.com/muhammadluth/goslogx/plugins/database/bun"

type Option func(*config)

type config struct {
	attributes []attribute.KeyValue
	statement  bool
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

func WithAttributes(attributes ...attribute.KeyValue) Option {
	copied := append([]attribute.KeyValue(nil), attributes...)
	return func(c *config) {
		for _, a := range copied {
			if a.Key == "db.namespace" && a.Value.Type() == attribute.STRING && len(a.Value.AsString()) <= 256 {
				c.attributes = append(c.attributes, a)
			}
		}
	}
}

// WithQueryStatement(false) preserves the privacy-safe default. Enabling capture
// returns a configuration error: the plugin does not collect SQL or arguments.
func WithQueryStatement(enabled bool) Option {
	return func(c *config) { c.statement = enabled }
}

func Instrument(db *upbun.DB, provider goslogx.TelemetryProvider, opts ...Option) error {
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
	if c.statement {
		return errors.New("database instrumentation: SQL capture is unsupported; use application-controlled sanitized span attributes")
	}
	meter := mp.Meter(instrumentationScope)
	duration, err := meter.Float64Histogram("db.client.operation.duration", metric.WithUnit("s"), metric.WithDescription("Database operation duration."), metric.WithExplicitBucketBoundaries(.001, .005, .01, .05, .1, .5, 1, 5, 10))
	if err != nil {
		return err
	}
	poolAttributes := append(append([]attribute.KeyValue(nil), c.attributes...), attribute.String("db.client.connection.pool.name", c.poolName))
	if err := observePool(meter, db, poolAttributes); err != nil {
		return err
	}
	db.AddQueryHook(&queryHook{tracer: tp.Tracer(instrumentationScope), duration: duration, attributes: c.attributes})
	return nil
}

type queryHook struct {
	tracer     trace.Tracer
	duration   metric.Float64Histogram
	attributes []attribute.KeyValue
}

func (*queryHook) Init(*upbun.DB) {}

func (h *queryHook) BeforeQuery(ctx context.Context, event *upbun.QueryEvent) context.Context {
	operation := safeOperation(event.Operation())
	if operation == "" {
		operation = "QUERY"
	}
	ctx, _ = h.tracer.Start(ctx, safeOperation(operation), trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(h.spanAttributes(event, operation)...))
	return ctx
}

func (h *queryHook) AfterQuery(ctx context.Context, event *upbun.QueryEvent) {
	span := trace.SpanFromContext(ctx)
	attrs := h.spanAttributes(event, safeOperation(event.Operation()))
	if event.Err != nil && !errors.Is(event.Err, sql.ErrNoRows) && !errors.Is(event.Err, sql.ErrTxDone) {
		attrs = append(attrs, attribute.String("error.type", "database_error"))
		span.SetAttributes(attribute.String("error.type", "database_error"))
		span.SetStatus(codes.Error, "")
	}
	if h.duration != nil {
		h.duration.Record(ctx, time.Since(event.StartTime).Seconds(), metric.WithAttributes(attrs...))
	}
	span.End()
}

func (h *queryHook) spanAttributes(event *upbun.QueryEvent, operation string) []attribute.KeyValue {
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
