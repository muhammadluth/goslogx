// Package gorm configures the maintained GORM OpenTelemetry plugin with explicit providers.
package gorm

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	native "gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/database/gorm"

type Option = telemetry.Option

var WithDBName = telemetry.WithDBName
var WithStatementCapture = telemetry.WithStatementCapture
var WithStatementSanitizer = telemetry.WithStatementSanitizer
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

type timing struct {
	started time.Time
	op      string
}

func Instrument(db *native.DB, p goslogx.TelemetryProvider, opts ...Option) error {
	if db == nil || db.Config == nil || db.Dialector == nil {
		return errors.New("gorm instrumentation: nil database")
	}
	return db.Use(&plugin{provider: p, opts: append([]Option(nil), opts...)})
}

type plugin struct {
	provider goslogx.TelemetryProvider
	opts     []Option
}

func (*plugin) Name() string { return ScopeName }
func (p *plugin) Initialize(db *native.DB) error {
	if _, exists := db.Config.Plugins["otelgorm"]; exists {
		return errors.New("gorm instrumentation: OpenTelemetry plugin already installed")
	}
	system := "other_sql"
	switch db.Dialector.Name() {
	case "postgres":
		system = "postgresql"
	case "mysql":
		system = "mysql"
	case "sqlite":
		system = "sqlite"
	case "sqlserver":
		system = "microsoft.sql_server"
	}
	t, err := telemetry.New(p.provider, ScopeName, system, false, p.opts...)
	if err != nil {
		return err
	}
	tp, _, _ := telemetry.Resolve(p.provider, t.Config)
	formatter := func(query string) string {
		if t.Config.StatementCapture {
			text := t.Config.StatementSanitizer(query)
			if len(text) > 4096 {
				text = text[:4096]
			}
			return text
		}
		fields := strings.Fields(query)
		if len(fields) > 0 {
			return telemetry.DBOperation(fields[0])
		}
		return "QUERY"
	}
	if err := db.Use(tracing.NewPlugin(tracing.WithTracerProvider(telemetry.SafeDBProvider(tp, ScopeName, t.Config.StatementCapture)), tracing.WithAttributes(attribute.String("db.system.name", system), attribute.String("db.namespace", t.Config.Namespace)), tracing.WithoutMetrics(), tracing.WithoutQueryVariables(), tracing.WithQueryFormatter(formatter))); err != nil {
		return err
	}
	before := func(op string) func(*native.DB) {
		return func(tx *native.DB) { tx.InstanceSet(ScopeName+":timing", timing{time.Now(), op}) }
	}
	after := func(tx *native.DB) {
		if value, ok := tx.InstanceGet(ScopeName + ":timing"); ok {
			state := value.(timing)
			err := tx.Error
			if errors.Is(err, native.ErrRecordNotFound) {
				err = nil
			}
			t.RecordDatabase(tx.Statement.Context, state.op, time.Since(state.started), err)
		}
	}
	if err := db.Callback().Create().Before("*").Register(ScopeName+":before", before("INSERT")); err != nil {
		return err
	}
	if err := db.Callback().Create().After("*").Register(ScopeName+":after", after); err != nil {
		return err
	}
	if err := db.Callback().Query().Before("*").Register(ScopeName+":before", before("SELECT")); err != nil {
		return err
	}
	if err := db.Callback().Query().After("*").Register(ScopeName+":after", after); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("*").Register(ScopeName+":before", before("UPDATE")); err != nil {
		return err
	}
	if err := db.Callback().Update().After("*").Register(ScopeName+":after", after); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("*").Register(ScopeName+":before", before("DELETE")); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("*").Register(ScopeName+":after", after); err != nil {
		return err
	}
	if err := db.Callback().Raw().Before("*").Register(ScopeName+":before", before("EXEC")); err != nil {
		return err
	}
	if err := db.Callback().Raw().After("*").Register(ScopeName+":after", after); err != nil {
		return err
	}
	if err := db.Callback().Row().Before("*").Register(ScopeName+":before", before("QUERY")); err != nil {
		return err
	}
	if err := db.Callback().Row().After("*").Register(ScopeName+":after", after); err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return telemetry.SQLPool(t.Meter, sqlDB, t.Config.Namespace)
}
func Transaction(ctx context.Context, db *native.DB, p goslogx.TelemetryProvider, fn func(*native.DB) error, opts ...Option) error {
	if db == nil || fn == nil {
		return errors.New("gorm instrumentation: nil database or callback")
	}
	t, err := telemetry.New(p, ScopeName, "other_sql", false, opts...)
	if err != nil {
		return err
	}
	return t.Run(ctx, "BEGIN", trace.SpanKindClient, func(ctx context.Context) error { return db.WithContext(ctx).Transaction(fn) })
}
