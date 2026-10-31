package franzgo

import (
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

type Option = telemetry.Option
type config struct {
	telemetry.Config
	keyFormatter   func(*kgo.Record) (string, error)
	mergedConnects bool
}

func common(option telemetry.Option) Option { return option }
func WithDestinationNormalizer(fn func(string) string) Option {
	return common(telemetry.WithDestinationNormalizer(fn))
}
func WithConsumerGroup(value string) Option { return common(telemetry.WithConsumerGroup(value)) }
func WithClientID(value string) Option      { return common(telemetry.WithClientID(value)) }
func WithTraceAttributes(attrs ...attribute.KeyValue) Option {
	return common(telemetry.WithTraceAttributes(attrs...))
}
func WithMetricAttributes(attrs ...attribute.KeyValue) Option {
	return common(telemetry.WithMetricAttributes(attrs...))
}
func WithPropagator(p propagation.TextMapPropagator) Option {
	return common(telemetry.WithPropagator(p))
}
func WithTracing(enabled bool) Option       { return common(telemetry.WithTracing(enabled)) }
func WithMetrics(enabled bool) Option       { return common(telemetry.WithMetrics(enabled)) }
func WithConsumerLinks(enabled bool) Option { return common(telemetry.WithConsumerLinks(enabled)) }

// WithRecordKeyFormatter explicitly enables native key metadata for NewNative.
// Return a sanitized value or an error to omit the key. Keys never become metric labels.
func WithRecordKeyFormatter(format func(*kgo.Record) (string, error)) Option {
	return telemetry.PluginOption(func(c *config) { c.keyFormatter = format })
}

// WithMergedConnectsMetrics selects kotel's single connect counter with an outcome label.
func WithMergedConnectsMetrics(enabled bool) Option {
	return telemetry.PluginOption(func(c *config) { c.mergedConnects = enabled })
}
func configure(options []Option) config {
	var c config
	telemetry.ApplyOptions(&c.Config, &c, options...)
	return c
}
func (c config) keyAttributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	if c.keyFormatter == nil {
		return nil
	}
	for _, a := range attrs {
		if a.Key == "messaging.kafka.message.key" && a.Value.Type() == attribute.STRING && len(a.Value.AsString()) <= 4096 {
			return []attribute.KeyValue{a}
		}
	}
	return nil
}
