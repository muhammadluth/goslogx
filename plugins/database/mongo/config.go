package mongo

import (
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

type Option = telemetry.Option
type config struct {
	telemetry.Config
	formatter func(*event.CommandStartedEvent) string
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

// WithSpanNameFormatter maps the native command-event formatter. Return a safe
// operation label; commands can contain application payloads.
func WithSpanNameFormatter(f func(*event.CommandStartedEvent) string) Option {
	return telemetry.PluginOption(func(c *config) { c.formatter = f })
}
