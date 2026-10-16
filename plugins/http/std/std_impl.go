package std

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
const ScopeName = "github.com/muhammadluth/goslogx/plugins/http/std"

type Option func(*config)

type config struct {
	serverName                       string
	options                          []otelhttp.Option
	attributes                       []attribute.KeyValue
	route                            string
	propagator                       propagation.TextMapPropagator
	tracingDisabled, metricsDisabled bool
	traceAttributes                  []attribute.KeyValue
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
			if (a.Key == "deployment.environment.name" || a.Key == "service.version" || a.Key == "app.channel" || a.Key == "app.region" || a.Key == "app.tier") && a.Value.Type() == attribute.STRING && len(a.Value.AsString()) <= 128 {
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
		if r.Pattern != "" {
			return r.Pattern
		}
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
		if p := provider.TracerProvider(); !isNil(p) && !c.tracingDisabled {
			tp = p
		}
		if p := provider.MeterProvider(); !isNil(p) && !c.metricsDisabled {
			mp = p
		}
	}
	if c.propagator != nil {
		propagator = c.propagator
	}
	if c.serverName != "" {
		a := attribute.String("server.address", c.serverName)
		c.attributes = append(c.attributes, a)
		c.traceAttributes = append(c.traceAttributes, a)
	}
	if c.route != "" {
		c.attributes = append(c.attributes, attribute.String("http.route", c.route))
	}
	return append(c.options,
		otelhttp.WithTracerProvider(privateTracerProvider{TracerProvider: tp, route: c.route, attributes: c.traceAttributes}),
		otelhttp.WithMeterProvider(privateMeterProvider{MeterProvider: mp, attributes: c.attributes}),
		otelhttp.WithPropagators(propagator),
	)
}

type serverInstrumentationKey struct{}

func NewHandler(provider goslogx.TelemetryProvider, handler http.Handler, operation string, opts ...Option) http.Handler {
	observed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r.Pattern != "" && len(r.Pattern) <= 256 {
				trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("http.route", r.Pattern))
				labeler, _ := otelhttp.LabelerFromContext(r.Context())
				labeler.Add(attribute.String("http.route", r.Pattern))
			}
		}()
		child := r.WithContext(context.WithValue(r.Context(), serverInstrumentationKey{}, true))
		defer func() { r.Pattern = child.Pattern }()
		handler.ServeHTTP(w, child)
	})
	wrapped := otelhttp.NewHandler(observed, operation, options(provider, opts)...)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(serverInstrumentationKey{}) == true {
			handler.ServeHTTP(w, r)
			return
		}
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

// InstrumentClient returns a shallow copy and retains timeout, redirects and cookies.
func InstrumentClient(client *http.Client, p goslogx.TelemetryProvider, opts ...Option) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	copy.Transport = NewTransport(p, client.Transport, opts...)
	return &copy
}
func WithTracing(enabled bool) Option { return func(c *config) { c.tracingDisabled = !enabled } }
func WithMetrics(enabled bool) Option { return func(c *config) { c.metricsDisabled = !enabled } }
func WithSpanStartOptions(opts ...trace.SpanStartOption) Option {
	copy := append([]trace.SpanStartOption(nil), opts...)
	return func(c *config) {
		c.options = append(c.options, otelhttp.WithSpanOptions(copy...))
		cfg := trace.NewSpanStartConfig(copy...)
		c.traceAttributes = append(c.traceAttributes, cfg.Attributes()...)
	}
}
func WithTraceAttributes(attrs ...attribute.KeyValue) Option {
	copy := append([]attribute.KeyValue(nil), attrs...)
	return func(c *config) { c.traceAttributes = append(c.traceAttributes, copy...) }
}
func WithPublicEndpoint() Option {
	return func(c *config) {
		c.options = append(c.options, otelhttp.WithPublicEndpointFn(func(*http.Request) bool { return true }))
	}
}

// WithServerName supplies a static server.address; request Host is never used as a label.
func WithServerName(name string) Option {
	return func(c *config) {
		if len(name) <= 256 {
			c.serverName = name
		}
	}
}
