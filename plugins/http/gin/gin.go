// Package gin adapts official otelgin middleware to explicit goslogx providers.
package gin

import (
	"github.com/gin-gonic/gin"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/http/gin"

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

var WithSpanStartOptions = httptelemetry.WithSpanStartOptions

// Formatter receives the registered template, never a path supplied as its first argument.
var WithSpanNameFormatter = httptelemetry.WithSpanNameFormatter

func Middleware(provider goslogx.TelemetryProvider, opts ...Option) gin.HandlerFunc {
	c := httptelemetry.NewConfig(opts...)
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
	return otelgin.Middleware(c.ServerName, upstream...)
}
