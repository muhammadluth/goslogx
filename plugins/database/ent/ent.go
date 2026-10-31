// Package ent instruments Ent's public driver contract.
package ent

import (
	"context"
	"errors"
	"strings"

	"entgo.io/ent/dialect"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/trace"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/database/ent"

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

type Driver struct {
	dialect.Driver
	t *telemetry.Instrumentation
}

func WrapDriver(driver dialect.Driver, p goslogx.TelemetryProvider, opts ...Option) (*Driver, error) {
	if telemetry.Nil(driver) {
		return nil, errors.New("ent instrumentation: nil driver")
	}
	if _, ok := driver.(*Driver); ok {
		return nil, errors.New("ent instrumentation: driver already instrumented")
	}
	system := "other_sql"
	switch driver.Dialect() {
	case dialect.Postgres:
		system = "postgresql"
	case dialect.MySQL:
		system = "mysql"
	case dialect.SQLite:
		system = "sqlite"
	}
	t, err := telemetry.New(p, ScopeName, system, false, opts...)
	if err != nil {
		return nil, err
	}
	return &Driver{driver, t}, nil
}
func operation(query string) string {
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return "QUERY"
	}
	return telemetry.DBOperation(parts[0])
}
func (d *Driver) Exec(ctx context.Context, query string, args, v any) error {
	return d.t.Run(ctx, operation(query), trace.SpanKindClient, func(ctx context.Context) error { d.t.Statement(ctx, query); return d.Driver.Exec(ctx, query, args, v) })
}
func (d *Driver) Query(ctx context.Context, query string, args, v any) error {
	return d.t.Run(ctx, operation(query), trace.SpanKindClient, func(ctx context.Context) error { d.t.Statement(ctx, query); return d.Driver.Query(ctx, query, args, v) })
}
func (d *Driver) Tx(ctx context.Context) (dialect.Tx, error) {
	ctx, finish := d.t.Begin(ctx, "BEGIN", "", trace.SpanKindClient)
	tx, err := d.Driver.Tx(ctx)
	if err != nil {
		finish(err)
		return nil, err
	}
	return &transaction{Tx: tx, t: d.t, ctx: ctx, finish: finish}, nil
}

type transaction struct {
	dialect.Tx
	t      *telemetry.Instrumentation
	ctx    context.Context
	finish func(error)
}

func (t *transaction) Exec(ctx context.Context, query string, args, v any) error {
	ctx = trace.ContextWithSpan(ctx, trace.SpanFromContext(t.ctx))
	return t.t.Run(ctx, operation(query), trace.SpanKindClient, func(ctx context.Context) error { t.t.Statement(ctx, query); return t.Tx.Exec(ctx, query, args, v) })
}
func (t *transaction) Query(ctx context.Context, query string, args, v any) error {
	ctx = trace.ContextWithSpan(ctx, trace.SpanFromContext(t.ctx))
	return t.t.Run(ctx, operation(query), trace.SpanKindClient, func(ctx context.Context) error { t.t.Statement(ctx, query); return t.Tx.Query(ctx, query, args, v) })
}
func (t *transaction) Commit() error   { err := t.Tx.Commit(); t.finish(err); return err }
func (t *transaction) Rollback() error { err := t.Tx.Rollback(); t.finish(err); return err }

var _ dialect.Driver = (*Driver)(nil)
