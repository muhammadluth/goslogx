// Package echo adapts official otelecho middleware without replacing Echo error handling.
package echo

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/http/echo"

type config struct {
	common                 []httptelemetry.Option
	skipper                middleware.Skipper
	metricAttributesFn     otelecho.MetricAttributeFn
	echoMetricAttributesFn otelecho.EchoMetricAttributeFn
	onError                otelecho.OnErrorFn
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
func WithSkipper(skipper middleware.Skipper) Option {
	return func(c *config) { c.skipper = skipper }
}
func WithMetricAttributeFn(fn func(*http.Request) []attribute.KeyValue) Option {
	return func(c *config) {
		if fn != nil {
			c.metricAttributesFn = func(r *http.Request) []attribute.KeyValue {
				return httptelemetry.EncodeMetricAttributes(fn(r))
			}
		}
	}
}
func WithMetricAttributesFn(fn func(*http.Request) []attribute.KeyValue) Option {
	return WithMetricAttributeFn(fn)
}
func WithEchoMetricAttributeFn(fn func(echo.Context) []attribute.KeyValue) Option {
	return func(c *config) {
		if fn != nil {
			c.echoMetricAttributesFn = func(ctx echo.Context) []attribute.KeyValue {
				return httptelemetry.EncodeMetricAttributes(fn(ctx))
			}
		}
	}
}
func WithEchoMetricAttributesFn(fn func(echo.Context) []attribute.KeyValue) Option {
	return WithEchoMetricAttributeFn(fn)
}
func WithOnError(fn func(echo.Context, error)) Option {
	return func(c *config) { c.onError = fn }
}

func Middleware(provider goslogx.TelemetryProvider, opts ...Option) echo.MiddlewareFunc {
	local := config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&local)
		}
	}
	c := httptelemetry.NewConfig(local.common...)
	p := httptelemetry.Resolve(provider, ScopeName, c)
	upstream := []otelecho.Option{otelecho.WithTracerProvider(p.TP), otelecho.WithMeterProvider(p.MP), otelecho.WithPropagators(p.Prop)}
	if local.metricAttributesFn != nil {
		upstream = append(upstream, otelecho.WithMetricAttributeFn(local.metricAttributesFn))
	}
	if local.echoMetricAttributesFn != nil {
		upstream = append(upstream, otelecho.WithEchoMetricAttributeFn(local.echoMetricAttributesFn))
	}
	if local.onError != nil {
		upstream = append(upstream, otelecho.WithOnError(local.onError))
	}
	native := otelecho.Middleware(c.ServerName, upstream...)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		observed := native(next)
		return func(ctx echo.Context) error {
			if local.skipper != nil && local.skipper(ctx) {
				return next(ctx)
			}
			if c.Filter != nil && !c.Filter(ctx.Request()) {
				return next(ctx)
			}
			// otelecho dispatches the original application error handler before
			// recording the final status. Echo must not dispatch that error twice.
			_ = observed(ctx)
			return nil
		}
	}
}
