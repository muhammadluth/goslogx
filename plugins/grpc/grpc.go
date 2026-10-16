// Package grpc supplies official OTel gRPC stats handlers with explicit providers.
package grpc

import (
	"strings"

	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc/stats"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/grpc"

type config struct {
	signals                 telemetry.Config
	methods                 map[string]bool
	prop                    propagation.TextMapPropagator
	filter                  func(*stats.RPCTagInfo) bool
	public                  bool
	traceAttrs, metricAttrs []attribute.KeyValue
}
type Option func(*config)

func WithTracing(enabled bool) Option {
	return func(c *config) { c.signals.TracingDisabled = !enabled }
}
func WithMetrics(enabled bool) Option {
	return func(c *config) { c.signals.MetricsDisabled = !enabled }
}
func WithPropagator(p propagation.TextMapPropagator) Option {
	return func(c *config) {
		if !telemetry.Nil(p) {
			c.prop = p
		}
	}
}
func WithFilter(f func(*stats.RPCTagInfo) bool) Option { return func(c *config) { c.filter = f } }
func WithPublicEndpoint() Option                       { return func(c *config) { c.public = true } }

// WithMethodNames opts known, registered full methods into spans and dimensions.
// Without this option names are gRPC and raw service/method metadata is omitted.
func WithMethodNames(names ...string) Option {
	copy := append([]string(nil), names...)
	return func(c *config) {
		if c.methods == nil {
			c.methods = make(map[string]bool)
		}
		for _, name := range copy {
			name = strings.TrimPrefix(name, "/")
			parts := strings.Split(name, "/")
			if len(c.methods) < 64 && len(name) <= 256 && len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				c.methods[name] = true
			}
		}
	}
}
func WithTraceAttributes(attrs ...attribute.KeyValue) Option {
	copy := append([]attribute.KeyValue(nil), attrs...)
	return func(c *config) { c.traceAttrs = append(c.traceAttrs, copy...) }
}
func WithMetricAttributes(attrs ...attribute.KeyValue) Option {
	copy := append([]attribute.KeyValue(nil), attrs...)
	return func(c *config) {
		for _, a := range copy {
			switch a.Key {
			case "app.channel", "service.version", "deployment.environment.name":
				if a.Value.Type() == attribute.STRING && len(a.Value.AsString()) <= 128 {
					c.metricAttrs = append(c.metricAttrs, a)
				}
			}
		}
	}
}
func options(p goslogx.TelemetryProvider, opts []Option) []otelgrpc.Option {
	c := config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	tp, mp, prop := telemetry.Resolve(p, c.signals)
	if !telemetry.Nil(c.prop) {
		prop = c.prop
	}
	out := []otelgrpc.Option{otelgrpc.WithTracerProvider(privateTP{tp, c}), otelgrpc.WithMeterProvider(privateMP{mp, c}), otelgrpc.WithPropagators(prop)}
	if c.filter != nil {
		out = append(out, otelgrpc.WithFilter(c.filter))
	}
	if c.public {
		out = append(out, otelgrpc.WithPublicEndpoint())
	}
	return out
}
func NewServerHandler(p goslogx.TelemetryProvider, opts ...Option) stats.Handler {
	return otelgrpc.NewServerHandler(options(p, opts)...)
}
func NewClientHandler(p goslogx.TelemetryProvider, opts ...Option) stats.Handler {
	return otelgrpc.NewClientHandler(options(p, opts)...)
}
