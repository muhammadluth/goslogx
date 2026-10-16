// Package gorillamux adapts official otelmux middleware.
package gorillamux

import (
	"github.com/gorilla/mux"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/http/gorillamux"

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

var WithPublicEndpoint = httptelemetry.WithPublicEndpoint
var WithSpanNameFormatter = httptelemetry.WithSpanNameFormatter

func Middleware(provider goslogx.TelemetryProvider, opts ...Option) mux.MiddlewareFunc {
	c := httptelemetry.NewConfig(opts...)
	p := httptelemetry.Resolve(provider, ScopeName, c)
	upstream := []otelmux.Option{otelmux.WithTracerProvider(p.TP), otelmux.WithMeterProvider(p.MP), otelmux.WithPropagators(p.Prop)}
	if c.Filter != nil {
		upstream = append(upstream, otelmux.WithFilter(c.Filter))
	}
	if c.Public {
		upstream = append(upstream, otelmux.WithPublicEndpoint())
	}
	if c.SpanName != nil {
		upstream = append(upstream, otelmux.WithSpanNameFormatter(c.SpanName))
	}
	return otelmux.Middleware(c.ServerName, upstream...)
}
