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
	cfg := config{}
	telemetry.ApplyOptions(&cfg.Config, &cfg, p.opts...)
	t, err := telemetry.New(p.provider, ScopeName, system, false, func(c *telemetry.Config) { *c = cfg.Config })
	if err != nil {
		return err
	}
	tp, _, _ := telemetry.Resolve(p.provider, t.Config)
	formatter := func(query string) string {
		if t.Config.StatementCapture {
			text := query
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
	nativeOptions := []tracing.Option{tracing.WithTracerProvider(telemetry.ConfiguredDBProvider(tp, ScopeName, t.Config)), tracing.WithAttributes(attribute.String("db.system.name", system), attribute.String("db.namespace", t.Config.Namespace)), tracing.WithoutMetrics(), tracing.WithQueryFormatter(formatter)}
	if !cfg.queryVariables {
		nativeOptions = append(nativeOptions, tracing.WithoutQueryVariables())
	}
	if cfg.stackTrace {
		nativeOptions = append(nativeOptions, tracing.WithRecordStackTrace())
	}
	if cfg.serverAddressDisabled {
		nativeOptions = append(nativeOptions, tracing.WithoutServerAddress())
	}
	if err := db.Use(tracing.NewPlugin(nativeOptions...)); err != nil {
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
	registrations := []callbackRegistration{
		{db.Callback().Create().Before("*").Register, "before", before("INSERT")},
		{db.Callback().Create().After("*").Register, "after", after},
		{db.Callback().Query().Before("*").Register, "before", before("SELECT")},
		{db.Callback().Query().After("*").Register, "after", after},
		{db.Callback().Update().Before("*").Register, "before", before("UPDATE")},
		{db.Callback().Update().After("*").Register, "after", after},
		{db.Callback().Delete().Before("*").Register, "before", before("DELETE")},
		{db.Callback().Delete().After("*").Register, "after", after},
		{db.Callback().Raw().Before("*").Register, "before", before("EXEC")},
		{db.Callback().Raw().After("*").Register, "after", after},
		{db.Callback().Row().Before("*").Register, "before", before("QUERY")},
		{db.Callback().Row().After("*").Register, "after", after},
	}
	for _, registration := range registrations {
		if err := registration.register(ScopeName+":"+registration.phase, registration.hook); err != nil {
			return err
		}
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
	cfg := config{}
	telemetry.ApplyOptions(&cfg.Config, &cfg, opts...)
	t, err := telemetry.New(p, ScopeName, "other_sql", false, func(c *telemetry.Config) { *c = cfg.Config })
	if err != nil {
		return err
	}
	return t.Run(ctx, "BEGIN", trace.SpanKindClient, func(ctx context.Context) error { return db.WithContext(ctx).Transaction(fn) })
}

type callbackRegistration struct {
	register func(string, func(*native.DB)) error
	phase    string
	hook     func(*native.DB)
}
