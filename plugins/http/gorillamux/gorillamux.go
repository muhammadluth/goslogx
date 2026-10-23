// Package gorillamux adapts official otelmux middleware.
package gorillamux

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	"go.opentelemetry.io/otel/attribute"
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
var WithPublicEndpointFn = httptelemetry.WithPublicEndpointFn
var WithSpanNameFormatter = httptelemetry.WithSpanNameFormatter
var WithMetricAttributesFn = httptelemetry.WithMetricAttributesFn

func Middleware(provider goslogx.TelemetryProvider, opts ...Option) mux.MiddlewareFunc {
	c := httptelemetry.NewConfig(opts...)
	p := httptelemetry.Resolve(provider, ScopeName, c)
	upstream := []otelmux.Option{otelmux.WithTracerProvider(p.TP), otelmux.WithMeterProvider(p.MP), otelmux.WithPropagators(p.Prop)}
	if c.Filter != nil {
		upstream = append(upstream, otelmux.WithFilter(c.Filter))
	}
	if c.Public {
		upstream = append(upstream, otelmux.WithPublicEndpoint())
	} else if c.PublicEndpointFn != nil {
		upstream = append(upstream, otelmux.WithPublicEndpointFn(c.PublicEndpointFn))
	}
	if c.SpanName != nil {
		upstream = append(upstream, otelmux.WithSpanNameFormatter(c.SpanName))
	}
	if c.MetricAttributesFn != nil {
		upstream = append(upstream, otelmux.WithMetricAttributesFn(func(r *http.Request) []attribute.KeyValue {
			return httptelemetry.EncodeMetricAttributes(c.MetricAttributesFn(r))
		}))
	}
	return otelmux.Middleware(c.ServerName, upstream...)
}
