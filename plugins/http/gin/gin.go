// Package gin adapts official otelgin middleware to explicit goslogx providers.
package gin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/http/gin"

type config struct {
	common                []httptelemetry.Option
	ginFilters            []otelgin.GinFilter
	metricAttributesFn    otelgin.MetricAttributeFn
	ginMetricAttributesFn otelgin.GinMetricAttributeFn
}

type Option func(*config)

func common(option httptelemetry.Option) Option {
	return func(c *config) { c.common = append(c.common, option) }
}

func WithServerName(name string) Option  { return common(httptelemetry.WithServerName(name)) }
func WithServiceName(name string) Option { return WithServerName(name) }
func WithFilter(filter func(*http.Request) bool) Option {
	return common(httptelemetry.WithFilter(filter))
}
func WithRequestFilter(filter func(*http.Request) bool) Option { return WithFilter(filter) }
func WithGinFilter(filters ...func(*gin.Context) bool) Option {
	return func(c *config) {
		for _, filter := range filters {
			if filter != nil {
				c.ginFilters = append(c.ginFilters, filter)
			}
		}
	}
}
func WithTraceAttributes(attrs ...attribute.KeyValue) Option {
	return common(httptelemetry.WithTraceAttributes(attrs...))
}
func WithMetricAttributes(attrs ...attribute.KeyValue) Option {
	return common(httptelemetry.WithMetricAttributes(attrs...))
}
func WithPropagator(p propagation.TextMapPropagator) Option {
	return common(httptelemetry.WithPropagator(p))
}
func WithTracing(enabled bool) Option { return common(httptelemetry.WithTracing(enabled)) }
func WithMetrics(enabled bool) Option { return common(httptelemetry.WithMetrics(enabled)) }
func WithSpanStartOptions(opts ...trace.SpanStartOption) Option {
	return common(httptelemetry.WithSpanStartOptions(opts...))
}
func WithSpanNameFormatter(formatter func(string, *http.Request) string) Option {
	return common(httptelemetry.WithSpanNameFormatter(formatter))
}
func WithMetricAttributesFn(fn func(*http.Request) []attribute.KeyValue) Option {
	return func(c *config) {
		if fn != nil {
			c.metricAttributesFn = func(r *http.Request) []attribute.KeyValue {
				return httptelemetry.EncodeMetricAttributes(fn(r))
			}
		}
	}
}
func WithMetricAttributeFn(fn func(*http.Request) []attribute.KeyValue) Option {
	return WithMetricAttributesFn(fn)
}
func WithGinMetricAttributesFn(fn func(*gin.Context) []attribute.KeyValue) Option {
	return func(c *config) {
		if fn != nil {
			c.ginMetricAttributesFn = func(ctx *gin.Context) []attribute.KeyValue {
				return httptelemetry.EncodeMetricAttributes(fn(ctx))
			}
		}
	}
}
func WithGinMetricAttributeFn(fn func(*gin.Context) []attribute.KeyValue) Option {
	return WithGinMetricAttributesFn(fn)
}

func Middleware(provider goslogx.TelemetryProvider, opts ...Option) gin.HandlerFunc {
	local := config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&local)
		}
	}
	c := httptelemetry.NewConfig(local.common...)
	p := httptelemetry.Resolve(provider, ScopeName, c)
	upstream := []otelgin.Option{otelgin.WithTracerProvider(p.TP), otelgin.WithMeterProvider(p.MP), otelgin.WithPropagators(p.Prop), otelgin.WithSpanNameFormatter(func(ctx *gin.Context) string {
		if c.SpanName != nil {
			return c.SpanName(ctx.FullPath(), ctx.Request)
		}
		return httptelemetry.Name(ctx.Request.Method, ctx.FullPath())
	})}
	if c.Filter != nil {
		upstream = append(upstream, otelgin.WithFilter(c.Filter))
	}
	if len(local.ginFilters) != 0 {
		upstream = append(upstream, otelgin.WithGinFilter(local.ginFilters...))
	}
	if local.metricAttributesFn != nil {
		upstream = append(upstream, otelgin.WithMetricAttributeFn(local.metricAttributesFn))
	}
	if local.ginMetricAttributesFn != nil {
		upstream = append(upstream, otelgin.WithGinMetricAttributeFn(local.ginMetricAttributesFn))
	}
	return otelgin.Middleware(c.ServerName, upstream...)
}
