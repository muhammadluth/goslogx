package http

import (
	"context"
	"io"
	"net/http"
	"reflect"

	"github.com/muhammadluth/goslogx"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// ScopeName identifies the integration without request-specific values.
const ScopeName = "github.com/muhammadluth/goslogx/plugins/http"

type Option func(*config)

type config struct {
	options    []otelhttp.Option
	attributes []attribute.KeyValue
	route      string
	propagator propagation.TextMapPropagator
}

// WithRoute records a static route template, such as /payments/{id}.
// Call it when registering a handler; never pass a request URL.
func WithRoute(route string) Option {
	return func(c *config) {
		if route != "" && len(route) <= 256 {
			c.route = route
		}
	}
}

// WithPropagator overrides the provider propagator with trusted application configuration.
func WithPropagator(p propagation.TextMapPropagator) Option {
	return func(c *config) {
		if !isNil(p) {
			c.propagator = p
		}
	}
}

func WithRequestFilter(filter func(*http.Request) bool) Option {
	return func(c *config) {
		if filter != nil {
			c.options = append(c.options, otelhttp.WithFilter(filter))
		}
	}
}

func WithSpanNameFormatter(formatter func(string, *http.Request) string) Option {
	return func(c *config) {
		if formatter != nil {
			c.options = append(c.options, otelhttp.WithSpanNameFormatter(formatter))
		}
	}
}

func WithMetricAttributes(attributes ...attribute.KeyValue) Option {
	copied := append([]attribute.KeyValue(nil), attributes...)
	return func(c *config) {
		for _, a := range copied {
			if (a.Key == "deployment.environment.name" || a.Key == "service.version") && a.Value.Type() == attribute.STRING && len(a.Value.AsString()) <= 128 {
				c.attributes = append(c.attributes, a)
			}
		}
	}
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func options(provider goslogx.TelemetryProvider, opts []Option) []otelhttp.Option {
	c := config{options: []otelhttp.Option{otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
		if operation != "" {
			return operation
		}
		method := safeAttributes([]attribute.KeyValue{attribute.String("http.request.method", r.Method)})
		if len(method) == 0 {
			return "HTTP _OTHER"
		}
		return "HTTP " + method[0].Value.AsString()
	})}}
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	var propagator propagation.TextMapPropagator = propagation.TraceContext{}
	var tp trace.TracerProvider = tracenoop.NewTracerProvider()
	var mp metric.MeterProvider = metricnoop.NewMeterProvider()
	if !isNil(provider) {
		if p := provider.TextMapPropagator(); !isNil(p) {
			propagator = p
		}
		if p := provider.TracerProvider(); !isNil(p) {
			tp = p
		}
		if p := provider.MeterProvider(); !isNil(p) {
			mp = p
		}
	}
	if c.propagator != nil {
		propagator = c.propagator
	}
	if c.route != "" {
		c.attributes = append(c.attributes, attribute.String("http.route", c.route))
	}
	return append(c.options,
		otelhttp.WithTracerProvider(privateTracerProvider{TracerProvider: tp, route: c.route}),
		otelhttp.WithMeterProvider(privateMeterProvider{MeterProvider: mp, attributes: c.attributes}),
		otelhttp.WithPropagators(propagator),
	)
}

func NewHandler(provider goslogx.TelemetryProvider, handler http.Handler, operation string, opts ...Option) http.Handler {
	wrapped := otelhttp.NewHandler(handler, operation, options(provider, opts)...)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped.ServeHTTP(w, r.Clone(r.Context()))
	})
}

func NewTransport(provider goslogx.TelemetryProvider, base http.RoundTripper, opts ...Option) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return transport{base: otelhttp.NewTransport(replayTransport{base: base}, options(provider, opts)...)}
}

type transport struct{ base http.RoundTripper }

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := r.Context()
	if r.GetBody != nil {
		ctx = context.WithValue(ctx, replayKey{}, r.GetBody)
	}
	cloned := r.Clone(ctx)
	cloned.GetBody = nil
	return t.base.RoundTrip(cloned)
}

// Keep replay available to the application transport, while preventing otelhttp
// itself from replaying bodies and reporting raw replay errors globally.
type replayKey struct{}
type replayTransport struct{ base http.RoundTripper }

func (t replayTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if getBody, ok := r.Context().Value(replayKey{}).(func() (io.ReadCloser, error)); ok {
		r = r.Clone(r.Context())
		r.GetBody = getBody
	}
	return t.base.RoundTrip(r)
}
