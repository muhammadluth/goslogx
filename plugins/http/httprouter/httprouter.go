// Package httprouter instruments native handlers using their registered templates.
package httprouter

import (
	"context"
	"github.com/julienschmidt/httprouter"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry"
	base "github.com/muhammadluth/goslogx/plugins/http/std"
	"net/http"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/http/httprouter"

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
var WithSpanNameFormatter = httptelemetry.WithSpanNameFormatter
var WithMetricAttributesFn = httptelemetry.WithMetricAttributesFn
var WithMessageEvents = httptelemetry.WithMessageEvents

const (
	ReadEvent  = httptelemetry.ReadEvent
	WriteEvent = httptelemetry.WriteEvent
)

type paramsKey struct{}

// Fallback instruments a NotFound or MethodNotAllowed handler. Install it on
// those router fields, not around the router, to avoid duplicate matched spans.
// An empty route prevents unmatched request paths becoming metric dimensions.
func Fallback(provider goslogx.TelemetryProvider, next http.Handler, opts ...Option) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	h := Handle(provider, "", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		next.ServeHTTP(w, r)
	}, opts...)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h(w, r, nil) })
}

// Handle wraps a native handler at registration; pattern must be the same static
// template passed to Router.Handle. Params remain available without reflection.
func Handle(provider goslogx.TelemetryProvider, pattern string, next httprouter.Handle, opts ...Option) httprouter.Handle {
	c := httptelemetry.NewConfig(opts...)
	p := httptelemetry.ResolveForDelegation(provider, ScopeName, c)
	if len(pattern) > 256 {
		pattern = ""
	}
	options := []base.Option{base.WithRoute(pattern), base.WithRequestFilter(c.Filter), base.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		if c.SpanName != nil {
			return c.SpanName(pattern, r)
		}
		return httptelemetry.Name(r.Method, pattern)
	})}
	options = append(options, base.WithTraceAttributes(c.TraceAttributes...), base.WithMetricAttributes(c.MetricAttributes...), base.WithSpanStartOptions(c.Start...), base.WithServerName(c.ServerName))
	if c.Public {
		options = append(options, base.WithPublicEndpoint())
	} else if c.PublicEndpointFn != nil {
		options = append(options, base.WithPublicEndpointFn(c.PublicEndpointFn))
	}
	if c.MetricAttributesFn != nil {
		options = append(options, base.WithMetricAttributesFn(c.MetricAttributesFn))
	}
	if len(c.MessageEvents) != 0 {
		events := make([]base.MessageEvent, 0, len(c.MessageEvents))
		for _, event := range c.MessageEvents {
			switch event {
			case httptelemetry.ReadEvent:
				events = append(events, base.ReadEvent)
			case httptelemetry.WriteEvent:
				events = append(events, base.WriteEvent)
			}
		}
		options = append(options, base.WithMessageEvents(events...))
	}
	h := base.NewHandler(p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params, _ := r.Context().Value(paramsKey{}).(httprouter.Params)
		next(w, r, params)
	}), "HTTP", options...)
	return func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), paramsKey{}, params)))
	}
}
