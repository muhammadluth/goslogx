// Package httptelemetry shares provider resolution and privacy policies across HTTP adapters.
package httptelemetry

import (
	"github.com/muhammadluth/goslogx"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"net/http"
	"reflect"
	"strings"
)

type Option func(*Config)
type MessageEvent int

const (
	ReadEvent MessageEvent = iota
	WriteEvent
)

type Config struct {
	ServerName                               string
	Filter                                   func(*http.Request) bool
	PublicEndpointFn                         func(*http.Request) bool
	MetricAttributesFn                       func(*http.Request) []attribute.KeyValue
	MessageEvents                            []MessageEvent
	SpanName                                 func(string, *http.Request) string
	TraceAttributes, MetricAttributes        []attribute.KeyValue
	Start                                    []trace.SpanStartOption
	Propagator                               propagation.TextMapPropagator
	TracingDisabled, MetricsDisabled, Public bool
}

func NewConfig(opts ...Option) Config {
	c := Config{}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	return c
}
func WithServerName(s string) Option {
	return func(c *Config) {
		if len(s) <= 256 {
			c.ServerName = strings.Clone(s)
		}
	}
}
func WithFilter(f func(*http.Request) bool) Option { return func(c *Config) { c.Filter = f } }
func WithPublicEndpointFn(f func(*http.Request) bool) Option {
	return func(c *Config) { c.PublicEndpointFn = f }
}
func WithMetricAttributesFn(f func(*http.Request) []attribute.KeyValue) Option {
	return func(c *Config) { c.MetricAttributesFn = f }
}
func WithMessageEvents(events ...MessageEvent) Option {
	copy := append([]MessageEvent(nil), events...)
	return func(c *Config) { c.MessageEvents = append(c.MessageEvents, copy...) }
}
func WithSpanNameFormatter(f func(string, *http.Request) string) Option {
	return func(c *Config) { c.SpanName = f }
}
func WithTraceAttributes(a ...attribute.KeyValue) Option {
	copy := append([]attribute.KeyValue(nil), a...)
	return func(c *Config) { c.TraceAttributes = append(c.TraceAttributes, copy...) }
}
func WithMetricAttributes(a ...attribute.KeyValue) Option {
	copy := append([]attribute.KeyValue(nil), a...)
	return func(c *Config) {
		for _, v := range copy {
			switch v.Key {
			case "app.channel", "app.region", "app.tier", "service.version", "deployment.environment.name":
				if v.Value.Type() == attribute.STRING && len(v.Value.AsString()) <= 128 {
					c.MetricAttributes = append(c.MetricAttributes, v)
				}
			}
		}
	}
}
func WithSpanStartOptions(opts ...trace.SpanStartOption) Option {
	copy := append([]trace.SpanStartOption(nil), opts...)
	return func(c *Config) { c.Start = append(c.Start, copy...) }
}
func WithPropagator(p propagation.TextMapPropagator) Option {
	return func(c *Config) {
		if !Nil(p) {
			c.Propagator = p
		}
	}
}
func WithTracing(v bool) Option  { return func(c *Config) { c.TracingDisabled = !v } }
func WithMetrics(v bool) Option  { return func(c *Config) { c.MetricsDisabled = !v } }
func WithPublicEndpoint() Option { return func(c *Config) { c.Public = true } }
func Nil(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Func, reflect.Slice, reflect.Chan:
		return r.IsNil()
	}
	return false
}

type Provider struct {
	TP   trace.TracerProvider
	MP   metric.MeterProvider
	Prop propagation.TextMapPropagator
}

func (p Provider) TracerProvider() trace.TracerProvider             { return p.TP }
func (p Provider) MeterProvider() metric.MeterProvider              { return p.MP }
func (p Provider) TextMapPropagator() propagation.TextMapPropagator { return p.Prop }
func Resolve(p goslogx.TelemetryProvider, scope string, c Config) Provider {
	var tp trace.TracerProvider = tracenoop.NewTracerProvider()
	var mp metric.MeterProvider = metricnoop.NewMeterProvider()
	var prop propagation.TextMapPropagator = propagation.TraceContext{}
	if !Nil(p) {
		if v := p.TracerProvider(); !Nil(v) && !c.TracingDisabled {
			tp = v
		}
		if v := p.MeterProvider(); !Nil(v) && !c.MetricsDisabled {
			mp = v
		}
		if v := p.TextMapPropagator(); !Nil(v) {
			prop = v
		}
	}
	if !Nil(c.Propagator) {
		prop = c.Propagator
	}
	traceAttrs := append([]attribute.KeyValue(nil), c.TraceAttributes...)
	metricAttrs := append([]attribute.KeyValue(nil), c.MetricAttributes...)
	if c.ServerName != "" {
		a := attribute.String("server.address", c.ServerName)
		traceAttrs = append(traceAttrs, a)
		metricAttrs = append(metricAttrs, a)
	}
	return Provider{TP: privateTracerProvider{TracerProvider: tp, scope: scope, start: c.Start, attributes: traceAttrs}, MP: privateMeterProvider{MeterProvider: mp, scope: scope, attributes: metricAttrs}, Prop: prop}
}
func Method(s string) string {
	switch s {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE", "CONNECT":
		return s
	}
	return "_OTHER"
}
func Name(method, route string) string {
	method = Method(method)
	if route != "" && len(route) <= 256 {
		return method + " " + route
	}
	return method
}

// ResolveForDelegation supplies provider ownership and scope for adapters that
// delegate to the std plugin. The std boundary applies the privacy policy once;
// applying it twice would discard already-validated custom attributes/events.
// The caller must forward Config options to the delegated adapter.
func ResolveForDelegation(p goslogx.TelemetryProvider, scope string, c Config) Provider {
	resolved := Resolve(p, scope, c)
	tp := resolved.TP.(privateTracerProvider)
	mp := resolved.MP.(privateMeterProvider)
	resolved.TP = scopeTracerProvider{TracerProvider: tp.TracerProvider, scope: scope}
	resolved.MP = scopeMeterProvider{MeterProvider: mp.MeterProvider, scope: scope}
	return resolved
}

type scopeTracerProvider struct {
	trace.TracerProvider
	scope string
}

func (p scopeTracerProvider) Tracer(_ string, opts ...trace.TracerOption) trace.Tracer {
	return p.TracerProvider.Tracer(p.scope, opts...)
}

type scopeMeterProvider struct {
	metric.MeterProvider
	scope string
}

func (p scopeMeterProvider) Meter(_ string, opts ...metric.MeterOption) metric.Meter {
	return p.MeterProvider.Meter(p.scope, opts...)
}
