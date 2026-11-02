// Package sql instruments database/sql drivers without depending on a driver.
package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"

	"github.com/XSAM/otelsql"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/privacy"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/database/sql"

func options(p goslogx.TelemetryProvider, system string, opts []Option) ([]otelsql.Option, config, error) {
	cfg := defaultConfig()
	telemetry.ApplyOptions(&cfg.Config, &cfg, opts...)
	if cfg.StatementCapture && cfg.StatementSanitizer == nil && !cfg.RawStatement {
		return nil, cfg, errors.New("sql instrumentation: statement capture requires a sanitizer")
	}
	tp, mp, prop := telemetry.Resolve(p, cfg.Config)
	attrs := []attribute.KeyValue{attribute.String("db.system.name", system)}
	if cfg.Namespace != "" {
		attrs = append(attrs, attribute.String("db.namespace", cfg.Namespace))
	}
	safeError := func(err error) []attribute.KeyValue {
		if err == nil || errors.Is(err, driver.ErrSkip) || errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return []attribute.KeyValue{attribute.String("error.type", privacy.ErrorType(err))}
	}
	upstream := []otelsql.Option{
		otelsql.WithTracerProvider(telemetry.ConfiguredDBProvider(tp, ScopeName, cfg.Config)), otelsql.WithMeterProvider(scopedMeterProvider{mp}), otelsql.WithTextMapPropagator(prop), otelsql.WithAttributes(append(attrs, cfg.MetricAttributes...)...),
		otelsql.WithSQLCommenter(cfg.commenter), otelsql.WithSpanOptions(cfg.spanOptions()),
		otelsql.WithSpanNameFormatter(func(ctx context.Context, method otelsql.Method, query string) string {
			if cfg.formatter != nil {
				return cfg.formatter(ctx, string(method), query)
			}
			return string(method)
		}),
		otelsql.WithInstrumentErrorAttributesGetter(safeError),
		otelsql.WithSpanErrorAttributesGetter(safeError),
		otelsql.WithAttributesGetter(func(_ context.Context, _ otelsql.Method, query string, _ []driver.NamedValue) []attribute.KeyValue {
			if !cfg.StatementCapture || query == "" {
				return nil
			}
			text := query
			if len(text) > 4096 {
				text = text[:4096]
			}
			return []attribute.KeyValue{attribute.String("db.query.text", text)}
		}),
	}
	return upstream, cfg, nil
}
func system(driverName string) string {
	switch strings.ToLower(driverName) {
	case "postgres", "pq", "pgx":
		return "postgresql"
	case "mysql":
		return "mysql"
	case "sqlite", "sqlite3":
		return "sqlite"
	case "sqlserver", "mssql":
		return "microsoft.sql_server"
	case "oracle", "godror":
		return "oracle"
	}
	return "other_sql"
}

// Open instruments the driver before opening the pool. It never records the DSN.
func Open(provider goslogx.TelemetryProvider, driverName, dsn string, opts ...Option) (*sql.DB, error) {
	upstream, cfg, err := options(provider, system(driverName), opts)
	if err != nil {
		return nil, err
	}
	db, err := otelsql.Open(driverName, dsn, upstream...)
	if err != nil {
		return nil, err
	}
	_ = cfg
	if _, err := otelsql.RegisterDBStatsMetrics(db, upstream...); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// OpenDB preserves the supplied connector, including its driver-specific behavior.
func OpenDB(provider goslogx.TelemetryProvider, connector driver.Connector, opts ...Option) (*sql.DB, error) {
	if telemetry.Nil(connector) {
		return nil, errors.New("sql instrumentation: nil connector")
	}
	upstream, cfg, err := options(provider, "other_sql", opts)
	if err != nil {
		return nil, err
	}
	db := otelsql.OpenDB(connector, upstream...)
	_ = cfg
	if _, err := otelsql.RegisterDBStatsMetrics(db, upstream...); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// WrapDriver permits driver-specific connectors while preserving optional interfaces.
func WrapDriver(provider goslogx.TelemetryProvider, d driver.Driver, opts ...Option) (driver.Driver, error) {
	if telemetry.Nil(d) {
		return nil, errors.New("sql instrumentation: nil driver")
	}
	upstream, _, err := options(provider, "other_sql", opts)
	if err != nil {
		return nil, err
	}
	return otelsql.WrapDriver(d, upstream...), nil
}

type scopedMeterProvider struct{ metric.MeterProvider }

func (p scopedMeterProvider) Meter(_ string, opts ...metric.MeterOption) metric.Meter {
	return p.MeterProvider.Meter(ScopeName, opts...)
}
