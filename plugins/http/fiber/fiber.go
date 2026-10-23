package fiber

import (
	"net/http"
	"reflect"
	"strings"

	"github.com/gofiber/contrib/v3/otel"
	"github.com/gofiber/fiber/v3"
	"github.com/muhammadluth/goslogx"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// ScopeName identifies the integration without request-specific values.
const ScopeName = "github.com/muhammadluth/goslogx/plugins/http/fiber"

type Option func(*config)

type config struct {
	options                          []otel.Option
	attributes                       []attribute.KeyValue
	serverName                       string
	propagator                       propagation.TextMapPropagator
	responsePropagator               propagation.TextMapPropagator
	tracingDisabled, metricsDisabled bool
	traceAttributes                  []attribute.KeyValue
	start                            []trace.SpanStartOption
	policy                           privacyPolicy
}

// WithRequestFilter accepts requests to instrument; false bypasses telemetry.
func WithRequestFilter(filter func(fiber.Ctx) bool) Option {
	return func(c *config) {
		if filter != nil {
			c.options = append(c.options, otel.WithNext(func(ctx fiber.Ctx) bool { return !filter(ctx) }))
		}
	}
}

// WithPropagator overrides the provider propagator. Avoid baggage containing secrets.
func WithPropagator(p propagation.TextMapPropagator) Option {
	return func(c *config) {
		if !isNil(p) {
			c.propagator = p
		}
	}
}

func WithNext(f func(fiber.Ctx) bool) Option {
	return func(c *config) {
		if f != nil {
			c.options = append(c.options, otel.WithNext(f))
		}
	}
}

// WithTraceResponseHeader exposes the active trace ID in the named response
// header. It is disabled by default.
func WithTraceResponseHeader(header string) Option {
	return func(c *config) {
		header = strings.TrimSpace(header)
		if header != "" && len(header) <= 128 && http.CanonicalHeaderKey(header) != "" {
			c.options = append(c.options, otel.WithTraceResponseHeader(header))
		}
	}
}

// WithPort sets a trusted static server.port on spans and metrics.
func WithPort(port int) Option {
	return func(c *config) {
		if port > 0 && port <= 65535 {
			c.options = append(c.options, otel.WithPort(port))
			c.traceAttributes = append(c.traceAttributes, attribute.Int("server.port", port))
			c.attributes = append(c.attributes, attribute.Int("server.port", port))
			c.policy.port = true
		}
	}
}

func WithCustomAttributes(fn func(fiber.Ctx) []attribute.KeyValue) Option {
	return func(c *config) {
		if fn != nil {
			c.options = append(c.options, otel.WithCustomAttributes(func(ctx fiber.Ctx) []attribute.KeyValue {
				return encodeTraceAttributes(fn(ctx))
			}))
		}
	}
}

func WithCustomMetricAttributes(fn func(fiber.Ctx) []attribute.KeyValue) Option {
	return func(c *config) {
		if fn != nil {
			c.options = append(c.options, otel.WithCustomMetricAttributes(func(ctx fiber.Ctx) []attribute.KeyValue {
				return encodeMetricAttributes(fn(ctx))
			}))
		}
	}
}

func WithCustomResponseAttributes(fn func(fiber.Ctx) []attribute.KeyValue) Option {
	return func(c *config) {
		if fn != nil {
			c.options = append(c.options, otel.WithCustomResponseAttributes(func(ctx fiber.Ctx) []attribute.KeyValue {
				return encodeTraceAttributes(fn(ctx))
			}))
		}
	}
}

func WithCustomResponseMetricAttributes(fn func(fiber.Ctx) []attribute.KeyValue) Option {
	return func(c *config) {
		if fn != nil {
			c.options = append(c.options, otel.WithCustomResponseMetricAttributes(func(ctx fiber.Ctx) []attribute.KeyValue {
				return encodeMetricAttributes(fn(ctx))
			}))
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

func WithServerName(name string) Option {
	return func(c *config) {
		if name != "" && len(name) <= 256 {
			c.serverName = name
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

func options(provider goslogx.TelemetryProvider, opts []Option) []otel.Option {
	c := config{options: []otel.Option{
		otel.WithSpanNameFormatter(func(ctx fiber.Ctx) string {
			name := method(ctx.Method())
			if ctx.Matched() && ctx.Route() != nil && len(ctx.Route().Handlers) > 0 {
				name += " " + ctx.Route().Path
			}
			return name
		}),
	}}
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
	responsePropagator := propagator
	if c.responsePropagator != nil {
		responsePropagator = c.responsePropagator
	}
	return append(c.options,
		otel.WithTracerProvider(privateTracerProvider{TracerProvider: tp, serverName: c.serverName, attributes: c.traceAttributes, start: c.start, policy: c.policy}),
		otel.WithMeterProvider(privateMeterProvider{MeterProvider: mp, attributes: c.attributes}),
		otel.WithPropagators(propagator),
		otel.WithResponsePropagators(responsePropagator),
	)
}

func NewMiddleware(provider goslogx.TelemetryProvider, opts ...Option) fiber.Handler {
	return otel.New(options(provider, opts)...)
}

// Middleware instruments a Fiber v3 server using explicit providers.
func Middleware(provider goslogx.TelemetryProvider, opts ...Option) fiber.Handler {
	return NewMiddleware(provider, opts...)
}

func WithTracing(enabled bool) Option { return func(c *config) { c.tracingDisabled = !enabled } }
func WithMetrics(enabled bool) Option { return func(c *config) { c.metricsDisabled = !enabled } }
func WithTraceAttributes(attrs ...attribute.KeyValue) Option {
	copy := append([]attribute.KeyValue(nil), attrs...)
	return func(c *config) { c.traceAttributes = append(c.traceAttributes, copy...) }
}
func WithSpanStartOptions(opts ...trace.SpanStartOption) Option {
	copy := append([]trace.SpanStartOption(nil), opts...)
	return func(c *config) { c.start = append(c.start, copy...) }
}
func WithSpanNameFormatter(f func(fiber.Ctx) string) Option {
	return func(c *config) {
		if f != nil {
			c.options = append(c.options, otel.WithSpanNameFormatter(f))
		}
	}
}

func WithPublicEndpoint() Option {
	return func(c *config) { c.options = append(c.options, otel.WithPublicEndpoint()) }
}

func WithPublicEndpointFn(fn func(fiber.Ctx) bool) Option {
	return func(c *config) {
		if fn != nil {
			c.options = append(c.options, otel.WithPublicEndpointFn(fn))
		}
	}
}

func WithResponsePropagator(p propagation.TextMapPropagator) Option {
	return func(c *config) {
		if !isNil(p) {
			c.responsePropagator = p
		}
	}
}

func WithCapturedRequestHeaders(headers ...string) Option {
	return capturedHeaders(true, headers...)
}

func WithCapturedResponseHeaders(headers ...string) Option {
	return capturedHeaders(false, headers...)
}

func capturedHeaders(request bool, headers ...string) Option {
	return func(c *config) {
		allowed := make([]string, 0, len(headers))
		if c.policy.headers == nil {
			c.policy.headers = make(map[attribute.Key]struct{})
		}
		for _, header := range headers {
			header = strings.TrimSpace(header)
			lower := strings.ToLower(header)
			if header == "" || len(header) > 128 || lower == "authorization" || lower == "proxy-authorization" || lower == "cookie" || lower == "set-cookie" {
				continue
			}
			prefix := "http.response.header."
			if request {
				prefix = "http.request.header."
			}
			c.policy.headers[attribute.Key(prefix+lower)] = struct{}{}
			allowed = append(allowed, header)
		}
		if request {
			c.options = append(c.options, otel.WithCapturedRequestHeaders(allowed...))
		} else {
			c.options = append(c.options, otel.WithCapturedResponseHeaders(allowed...))
		}
	}
}

func WithRedactedQueryParams(params ...string) Option {
	copy := append([]string(nil), params...)
	return func(c *config) { c.options = append(c.options, otel.WithRedactedQueryParams(copy...)) }
}

func WithClientIP(collect bool) Option {
	return func(c *config) {
		c.policy.clientIP = collect
		c.options = append(c.options, otel.WithClientIP(collect))
	}
}

// WithCollectClientIP is retained for compatibility with the upstream name.
// Deprecated: use WithClientIP.
func WithCollectClientIP(collect bool) Option { return WithClientIP(collect) }

// WithoutMetrics mirrors the upstream compatibility option.
func WithoutMetrics(disabled bool) Option { return WithMetrics(!disabled) }
