package sql

import (
	"context"
	"database/sql/driver"
	"github.com/XSAM/otelsql"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

type Option = telemetry.Option
type config struct {
	telemetry.Config
	spans     SpanOptions
	commenter bool
	formatter func(context.Context, string, string) string
	filter    func(context.Context, string, string, []driver.NamedValue) bool
}

// SpanOptions maps the supported driver lifecycle controls. Query capture is
// configured separately through WithStatementCapture / WithRawStatement.
type SpanOptions struct {
	Ping                 bool             `json:"ping"`
	RowsNext             bool             `json:"rows_next"`
	DisableErrSkip       bool             `json:"disable_err_skip"`
	OmitConnResetSession bool             `json:"omit_conn_reset_session"`
	OmitConnPrepare      bool             `json:"omit_conn_prepare"`
	OmitConnQuery        bool             `json:"omit_conn_query"`
	OmitRows             bool             `json:"omit_rows"`
	RowsChildOfQuery     bool             `json:"rows_child_of_query"`
	OmitConnectorConnect bool             `json:"omit_connector_connect"`
	RecordError          func(error) bool `json:"-"`
}

func defaultConfig() config {
	return config{spans: SpanOptions{Ping: true, DisableErrSkip: true, OmitConnResetSession: true, OmitRows: true}}
}
func (c config) spanOptions() otelsql.SpanOptions {
	o := otelSQLSpanOptions(c.spans)
	if c.filter != nil {
		o.SpanFilter = func(ctx context.Context, method otelsql.Method, query string, args []driver.NamedValue) bool {
			return c.filter(ctx, string(method), query, args)
		}
	}
	return o
}
func otelSQLSpanOptions(s SpanOptions) otelsql.SpanOptions {
	return otelsql.SpanOptions{Ping: s.Ping, RowsNext: s.RowsNext, DisableQuery: true, DisableErrSkip: s.DisableErrSkip, RecordError: s.RecordError, OmitConnResetSession: s.OmitConnResetSession, OmitConnPrepare: s.OmitConnPrepare, OmitConnQuery: s.OmitConnQuery, OmitRows: s.OmitRows, RowsChildOfQuery: s.RowsChildOfQuery, OmitConnectorConnect: s.OmitConnectorConnect}
}
func common(o telemetry.Option) Option   { return o }
func WithDBName(name string) Option      { return common(telemetry.WithDBName(name)) }
func WithRawStatement(v bool) Option     { return common(telemetry.WithRawStatement(v)) }
func WithStatementCapture(v bool) Option { return common(telemetry.WithStatementCapture(v)) }
func WithStatementSanitizer(f func(string) string) Option {
	return common(telemetry.WithStatementSanitizer(f))
}
func WithTracing(v bool) Option { return common(telemetry.WithTracing(v)) }
func WithMetrics(v bool) Option { return common(telemetry.WithMetrics(v)) }
func WithTraceAttributes(a ...attribute.KeyValue) Option {
	return common(telemetry.WithTraceAttributes(a...))
}
func WithMetricAttributes(a ...attribute.KeyValue) Option {
	return common(telemetry.WithMetricAttributes(a...))
}
func WithPropagator(p propagation.TextMapPropagator) Option {
	return common(telemetry.WithPropagator(p))
}
func WithSpanOptions(options SpanOptions) Option {
	return telemetry.PluginOption(func(c *config) { c.spans = options })
}
func WithSpanNameFormatter(f func(context.Context, string, string) string) Option {
	return telemetry.PluginOption(func(c *config) { c.formatter = f })
}
func WithFilter(f func(context.Context, string, string, []driver.NamedValue) bool) Option {
	return telemetry.PluginOption(func(c *config) { c.filter = f })
}

// WithSQLCommenter opts into modifying SQL with propagated trace context.
func WithSQLCommenter(enabled bool) Option {
	return telemetry.PluginOption(func(c *config) { c.commenter = enabled })
}
