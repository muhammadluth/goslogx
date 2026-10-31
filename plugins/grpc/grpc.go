// Package grpc supplies official OTel gRPC stats handlers with explicit providers.
package grpc

import (
	"context"
	"strings"

	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/stats"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/grpc"

type config struct {
	signals                 telemetry.Config
	methods                 map[string]bool
	prop                    propagation.TextMapPropagator
	filter                  func(*stats.RPCTagInfo) bool
	public                  bool
	publicFn                func(context.Context, *stats.RPCTagInfo) bool
	messageEvents           []otelgrpc.Event
	spanKind                trace.SpanKind
	spanKindSet             bool
	metricAttributesFn      func(context.Context) []attribute.KeyValue
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
func WithPublicEndpointFn(fn func(context.Context, *stats.RPCTagInfo) bool) Option {
	return func(c *config) { c.publicFn = fn }
}

// MessageEvent identifies a bounded gRPC message-size event. Payloads are never captured.
type MessageEvent int

const (
	ReceivedEvent MessageEvent = iota
	SentEvent
)

// WithMessageEvents enables per-message size events. It is disabled by default
// because streaming RPCs can produce many events.
func WithMessageEvents(events ...MessageEvent) Option {
	return func(c *config) {
		for _, event := range events {
			switch event {
			case ReceivedEvent:
				c.messageEvents = append(c.messageEvents, otelgrpc.ReceivedEvents)
			case SentEvent:
				c.messageEvents = append(c.messageEvents, otelgrpc.SentEvents)
			}
		}
	}
}

// WithSpanKind overrides the upstream client/server span kind explicitly.
func WithSpanKind(kind trace.SpanKind) Option {
	return func(c *config) { c.spanKind, c.spanKindSet = kind, true }
}

// WithMethodNames opts known, registered full methods into spans and dimensions.
// Span names and RPC attributes always preserve native otelgrpc semantics.
// This allowlist controls service/method metric dimensions only.
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

// WithSpanAttributes is the upstream-oriented alias of WithTraceAttributes.
func WithSpanAttributes(attrs ...attribute.KeyValue) Option { return WithTraceAttributes(attrs...) }
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

// WithMetricAttributesFn adds validated bounded attributes computed once per RPC.
func WithMetricAttributesFn(fn func(context.Context) []attribute.KeyValue) Option {
	return func(c *config) { c.metricAttributesFn = fn }
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
	} else if c.publicFn != nil {
		out = append(out, otelgrpc.WithPublicEndpointFn(c.publicFn))
	}
	if len(c.messageEvents) != 0 {
		out = append(out, otelgrpc.WithMessageEvents(c.messageEvents...))
	}
	if c.spanKindSet {
		out = append(out, otelgrpc.WithSpanKind(c.spanKind))
	}
	if c.metricAttributesFn != nil {
		out = append(out, otelgrpc.WithMetricAttributesFn(func(ctx context.Context) []attribute.KeyValue {
			return safeMetricAttributes(c.metricAttributesFn(ctx))
		}))
	}
	return out
}
func NewServerHandler(p goslogx.TelemetryProvider, opts ...Option) stats.Handler {
	return otelgrpc.NewServerHandler(options(p, opts)...)
}
func NewClientHandler(p goslogx.TelemetryProvider, opts ...Option) stats.Handler {
	return otelgrpc.NewClientHandler(options(p, opts)...)
}
