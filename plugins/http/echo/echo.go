// Package echo adapts official otelecho middleware without replacing Echo error handling.
package echo

import (
	"github.com/labstack/echo/v4"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/http/echo"

type Option = httptelemetry.Option

var WithServerName = httptelemetry.WithServerName
var WithServiceName = httptelemetry.WithServerName
var WithFilter = httptelemetry.WithFilter
var WithRequestFilter = httptelemetry.WithFilter
var WithTraceAttributes = httptelemetry.WithTraceAttributes
var WithMetricAttributes = httptelemetry.WithMetricAttributes
var WithPropagator = httptelemetry.WithPropagator
var WithTracing = httptelemetry.WithTracing
var WithMetrics = httptelemetry.WithMetrics

func Middleware(provider goslogx.TelemetryProvider, opts ...Option) echo.MiddlewareFunc {
	c := httptelemetry.NewConfig(opts...)
	p := httptelemetry.Resolve(provider, ScopeName, c)
	upstream := []otelecho.Option{otelecho.WithTracerProvider(p.TP), otelecho.WithMeterProvider(p.MP), otelecho.WithPropagators(p.Prop)}
	native := otelecho.Middleware(c.ServerName, upstream...)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		observed := native(next)
		return func(ctx echo.Context) error {
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
