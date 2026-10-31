package pgx

import (
	"context"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"time"
)

type Option = telemetry.Option
type config struct {
	telemetry.Config
	acquireDisabled           bool
	connectionDetailsDisabled bool
	prefix                    bool
	formatter                 func(context.Context, string) string
	parameters                func([]string) []string
	rawParameters             bool
	parameterFilter           func(string) bool
	poolStatsInterval         time.Duration
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

// WithAcquireTracing controls native pool acquire spans and their metrics.
func WithAcquireTracing(v bool) Option {
	return telemetry.PluginOption(func(c *config) { c.acquireDisabled = !v })
}

// WithConnectionDetails controls endpoint/database metadata, never credentials.
func WithConnectionDetails(v bool) Option {
	return telemetry.PluginOption(func(c *config) { c.connectionDetailsDisabled = !v })
}

// WithQuerySpanNamePrefix enables the upstream query/prepare/batch query prefixes.
func WithQuerySpanNamePrefix(v bool) Option {
	return telemetry.PluginOption(func(c *config) { c.prefix = v })
}

// WithSpanNameFormatter is for safe application operation names; metrics remain
// independently classified by SQL operation to avoid request-specific series.
func WithSpanNameFormatter(f func(context.Context, string) string) Option {
	return telemetry.PluginOption(func(c *config) { c.formatter = f })
}
func (c config) operation(ctx context.Context, sql string) string {
	if c.formatter != nil {
		return c.formatter(ctx, sql)
	}
	return operation(ctx, sql)
}
func (c config) name(ctx context.Context, sql, prefix string) string {
	name := c.operation(ctx, sql)
	if c.prefix {
		return prefix + name
	}
	return name
}

// WithQueryParameters enables parameter capture after application sanitization.
// A nil sanitizer leaves parameter capture disabled. Values are bounded before export.
func WithQueryParameters(sanitize func([]string) []string) Option {
	return telemetry.PluginOption(func(c *config) { c.parameters = sanitize })
}

// WithRawQueryParameters explicitly permits sensitive bind values for diagnostics.
func WithRawQueryParameters(enabled bool) Option {
	return telemetry.PluginOption(func(c *config) { c.rawParameters = enabled })
}

// WithQueryParameterFilter selects the statements whose parameters may be captured.
func WithQueryParameterFilter(filter func(string) bool) Option {
	return telemetry.PluginOption(func(c *config) { c.parameterFilter = filter })
}

// WithPoolStatsInterval sets the native minimum interval between pool statistics reads.
func WithPoolStatsInterval(interval time.Duration) Option {
	return telemetry.PluginOption(func(c *config) {
		if interval >= 0 {
			c.poolStatsInterval = interval
		}
	})
}

func (c config) parameterAttributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	if c.parameters == nil && !c.rawParameters {
		return nil
	}
	for _, a := range attrs {
		if a.Key != "pgx.query.parameters" || a.Value.Type() != attribute.STRINGSLICE {
			continue
		}
		values := append([]string(nil), a.Value.AsStringSlice()...)
		if c.parameters != nil {
			values = c.parameters(values)
		}
		if len(values) > 32 {
			values = values[:32]
		}
		remaining := 4096
		for i, value := range values {
			if len(value) > remaining {
				value = value[:remaining]
			}
			values[i] = value
			remaining -= len(value)
		}
		return []attribute.KeyValue{attribute.StringSlice("pgx.query.parameters", values)}
	}
	return nil
}
