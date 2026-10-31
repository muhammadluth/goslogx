package gorm

import (
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

type Option = telemetry.Option
type config struct {
	telemetry.Config
	stackTrace            bool
	queryVariables        bool
	serverAddressDisabled bool
}

func common(o telemetry.Option) Option   { return o }
func WithDBName(v string) Option         { return common(telemetry.WithDBName(v)) }
func WithRawStatement(v bool) Option     { return common(telemetry.WithRawStatement(v)) }
func WithStatementCapture(v bool) Option { return common(telemetry.WithStatementCapture(v)) }
func WithStatementSanitizer(f func(string) string) Option {
	return common(telemetry.WithStatementSanitizer(f))
}
func WithTraceAttributes(a ...attribute.KeyValue) Option {
	return common(telemetry.WithTraceAttributes(a...))
}
func WithMetricAttributes(a ...attribute.KeyValue) Option {
	return common(telemetry.WithMetricAttributes(a...))
}
func WithPropagator(p propagation.TextMapPropagator) Option {
	return common(telemetry.WithPropagator(p))
}
func WithTracing(v bool) Option { return common(telemetry.WithTracing(v)) }
func WithMetrics(v bool) Option { return common(telemetry.WithMetrics(v)) }

// WithRecordStackTrace enables upstream exception stack frames. Error text remains redacted.
func WithRecordStackTrace(enabled bool) Option {
	return telemetry.PluginOption(func(c *config) { c.stackTrace = enabled })
}

// WithServerAddress controls native server metadata on spans.
func WithServerAddress(enabled bool) Option {
	return telemetry.PluginOption(func(c *config) { c.serverAddressDisabled = !enabled })
}

// WithQueryVariables includes interpolated bind variables in the statement passed
// to the sanitizer. Statement capture must be enabled separately.
func WithQueryVariables(enabled bool) Option {
	return telemetry.PluginOption(func(c *config) { c.queryVariables = enabled })
}
