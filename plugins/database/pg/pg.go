package pg

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"time"

	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
	"github.com/muhammadluth/goslogx"

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

// WithQueryStatement(false) preserves the privacy-safe default. Enabling capture
// returns a configuration error: the plugin does not collect SQL or arguments.
func WithQueryStatement(enabled bool) Option {
	return func(c *config) { c.statement = enabled }
}

func Instrument(db *pg.DB, provider goslogx.TelemetryProvider, opts ...Option) error {
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

func (*queryHook) Init(*pg.DB) {}

func (h *queryHook) BeforeQuery(ctx context.Context, event *pg.QueryEvent) (context.Context, error) {
	operation := orm.QueryOp("")
	if query, ok := event.Query.(interface{ Operation() orm.QueryOp }); ok {
		operation = query.Operation()
	}
	name := safeOperation(string(operation))
	if name == "" {
		name = "QUERY"
	}
	ctx, _ = h.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(h.spanAttributes(event, operation)...))
	return ctx, nil
}

func (h *queryHook) AfterQuery(ctx context.Context, event *pg.QueryEvent) error {
	span := trace.SpanFromContext(ctx)
	operation := orm.QueryOp("")
	if query, ok := event.Query.(interface{ Operation() orm.QueryOp }); ok {
		operation = query.Operation()
	}
	attrs := h.spanAttributes(event, operation)
	if event.Err != nil && !errors.Is(event.Err, sql.ErrNoRows) && !errors.Is(event.Err, sql.ErrTxDone) && !errors.Is(event.Err, pg.ErrNoRows) && !errors.Is(event.Err, pg.ErrMultiRows) {
		attrs = append(attrs, attribute.String("error.type", "database_error"))
		span.SetAttributes(attribute.String("error.type", "database_error"))
		span.SetStatus(codes.Error, "")
	}
	if h.duration != nil {
		h.duration.Record(ctx, time.Since(event.StartTime).Seconds(), metric.WithAttributes(attrs...))
	}
	span.End()
	return nil
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
