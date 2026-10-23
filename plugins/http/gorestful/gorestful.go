// Package gorestful adapts official otelrestful tracing and supplies missing HTTP metrics.
package gorestful

import (
	"github.com/emicklei/go-restful/v3"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/emicklei/go-restful/otelrestful"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/http/gorestful"

type Option = httptelemetry.Option

var WithServerName = httptelemetry.WithServerName
var WithFilter = httptelemetry.WithFilter
var WithRequestFilter = httptelemetry.WithFilter
var WithTraceAttributes = httptelemetry.WithTraceAttributes
var WithMetricAttributes = httptelemetry.WithMetricAttributes
var WithPropagator = httptelemetry.WithPropagator
var WithTracing = httptelemetry.WithTracing
var WithMetrics = httptelemetry.WithMetrics
var WithSpanStartOptions = httptelemetry.WithSpanStartOptions

var WithPublicEndpoint = httptelemetry.WithPublicEndpoint
var WithPublicEndpointFn = httptelemetry.WithPublicEndpointFn

// NewFilter returns construction errors from metric creation to the caller.
func NewFilter(provider goslogx.TelemetryProvider, opts ...Option) (restful.FilterFunction, error) {
	c := httptelemetry.NewConfig(opts...)
	p := httptelemetry.Resolve(provider, ScopeName, c)
	metrics, err := httptelemetry.NewServerMetrics(p, c)
	if err != nil {
		return nil, err
	}
	upstream := []otelrestful.Option{otelrestful.WithTracerProvider(p.TP), otelrestful.WithPropagators(p.Prop)}
	if c.Public {
		upstream = append(upstream, otelrestful.WithPublicEndpoint())
	} else if c.PublicEndpointFn != nil {
		upstream = append(upstream, otelrestful.WithPublicEndpointFn(c.PublicEndpointFn))
	}
	filter := otelrestful.OTelFilter(c.ServerName, upstream...)
	return func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
		if c.Filter != nil && !c.Filter(req.Request) {
			chain.ProcessFilter(req, resp)
			return
		}
		finish := metrics.Begin(req.Request.Context(), req.Request.Method)
		defer func() { finish(req.SelectedRoutePath(), resp.StatusCode(), false) }()
		filter(req, resp, chain)
	}, nil
}
