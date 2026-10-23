// Package chi adds late route-template discovery to the standard HTTP adapter.
package chi

import (
	"github.com/go-chi/chi/v5"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry"
	base "github.com/muhammadluth/goslogx/plugins/http/std"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"net/http"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/http/chi"

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

func Middleware(provider goslogx.TelemetryProvider, opts ...Option) func(http.Handler) http.Handler {
	c := httptelemetry.NewConfig(opts...)
	p := httptelemetry.Resolve(provider, ScopeName, c)
	return func(next http.Handler) http.Handler {
		observed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				route := ""
				if rc := chi.RouteContext(r.Context()); rc != nil {
					route = rc.RoutePattern()
				}
				if len(route) > 256 {
					route = ""
				}
				if route != "" {
					span := trace.SpanFromContext(r.Context())
					span.SetAttributes(attribute.String("http.route", route))
					name := httptelemetry.Name(r.Method, route)
					if c.SpanName != nil {
						name = c.SpanName(route, r)
					}
					span.SetName(name)
					l, _ := otelhttp.LabelerFromContext(r.Context())
					l.Add(attribute.String("http.route", route))
				}
			}()
			next.ServeHTTP(w, r)
		})
		options := []base.Option{base.WithRequestFilter(c.Filter)}
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
		return base.NewHandler(p, observed, "HTTP", options...)
	}
}
