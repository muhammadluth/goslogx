package fiber

import (
	"reflect"

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
const ScopeName = "github.com/muhammadluth/goslogx/plugins/fiber"

type Option func(*config)

type config struct {
	options    []otel.Option
	attributes []attribute.KeyValue
	serverName string
	propagator propagation.TextMapPropagator
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
	return append(c.options,
		otel.WithTracerProvider(privateTracerProvider{TracerProvider: tp, serverName: c.serverName}),
		otel.WithMeterProvider(privateMeterProvider{MeterProvider: mp, attributes: c.attributes}),
		otel.WithPropagators(propagator),
		otel.WithResponsePropagators(propagator),
	)
}

func NewMiddleware(provider goslogx.TelemetryProvider, opts ...Option) fiber.Handler {
	return otel.New(options(provider, opts)...)
}

// Middleware instruments a Fiber v3 server using explicit providers.
func Middleware(provider goslogx.TelemetryProvider, opts ...Option) fiber.Handler {
	return NewMiddleware(provider, opts...)
}
