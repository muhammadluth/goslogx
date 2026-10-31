package dynamodb

import (
	"context"
	"github.com/aws/smithy-go/middleware"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

type Option = telemetry.Option
type config struct {
	telemetry.Config
	builders []otelaws.AttributeBuilder
}

func common(o telemetry.Option) Option { return o }
func WithDBName(v string) Option       { return common(telemetry.WithDBName(v)) }
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

// WithAttributeBuilder adds SDK-operation metadata to spans. The shared privacy
// policy filters its result; it does not affect metric dimensions or providers.
func WithAttributeBuilder(build func(context.Context, middleware.InitializeInput, middleware.InitializeOutput) []attribute.KeyValue) Option {
	return telemetry.PluginOption(func(c *config) {
		if build != nil {
			c.builders = append(c.builders, build)
		}
	})
}
