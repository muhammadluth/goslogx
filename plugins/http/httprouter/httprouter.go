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
var WithSpanNameFormatter = httptelemetry.WithSpanNameFormatter

type paramsKey struct{}

// Handle wraps a native handler at registration; pattern must be the same static
// template passed to Router.Handle. Params remain available without reflection.
func Handle(provider goslogx.TelemetryProvider, pattern string, next httprouter.Handle, opts ...Option) httprouter.Handle {
	c := httptelemetry.NewConfig(opts...)
	p := httptelemetry.Resolve(provider, ScopeName, c)
	if len(pattern) > 256 {
		pattern = ""
	}
	options := []base.Option{base.WithRoute(pattern), base.WithRequestFilter(c.Filter), base.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		if c.SpanName != nil {
			return c.SpanName(pattern, r)
		}
		return httptelemetry.Name(r.Method, pattern)
	})}
	if c.Public {
		options = append(options, base.WithPublicEndpoint())
	}
	h := base.NewHandler(p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params, _ := r.Context().Value(paramsKey{}).(httprouter.Params)
		next(w, r, params)
	}), "HTTP", options...)
	return func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), paramsKey{}, params)))
	}
}
