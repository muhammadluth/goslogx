// Package sqlx layers sqlx over the instrumented database/sql driver without duplicate spans.
package sqlx

import (
	"database/sql"
	"github.com/jmoiron/sqlx"
	"github.com/muhammadluth/goslogx"
	goslogxsql "github.com/muhammadluth/goslogx/plugins/database/sql"
)

type Option = goslogxsql.Option

var WithDBName = goslogxsql.WithDBName
var WithStatementCapture = goslogxsql.WithStatementCapture
var WithStatementSanitizer = goslogxsql.WithStatementSanitizer
var WithRawStatement = goslogxsql.WithRawStatement
var WithTraceAttributes = goslogxsql.WithTraceAttributes
var WithMetricAttributes = goslogxsql.WithMetricAttributes
var WithPropagator = goslogxsql.WithPropagator
var WithTracing = goslogxsql.WithTracing
var WithMetrics = goslogxsql.WithMetrics

func Open(p goslogx.TelemetryProvider, driverName, dsn string, opts ...Option) (*sqlx.DB, error) {
	db, err := goslogxsql.Open(p, driverName, dsn, opts...)
	if err != nil {
		return nil, err
	}
	return sqlx.NewDb(db, driverName), nil
}

// NewDb adds sqlx convenience methods to an already instrumented pool.
// It does not retrofit instrumentation onto an existing uninstrumented SQL driver.
func NewDb(db *sql.DB, driverName string) *sqlx.DB { return sqlx.NewDb(db, driverName) }

type SpanOptions = goslogxsql.SpanOptions

var WithSpanOptions = goslogxsql.WithSpanOptions
var WithSpanNameFormatter = goslogxsql.WithSpanNameFormatter
var WithFilter = goslogxsql.WithFilter
var WithSQLCommenter = goslogxsql.WithSQLCommenter
