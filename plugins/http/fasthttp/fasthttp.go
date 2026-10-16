// Package fasthttp provides native middleware with explicit providers and no net/http conversion.
package fasthttp

import (
	"context"
	"fmt"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/http/fasthttp"

type config struct {
	common httptelemetry.Config
	route  string
	filter func(*fasthttp.RequestCtx) bool
}
type Option func(*config)

func WithRoute(pattern string) Option {
	return func(c *config) {
		if len(pattern) <= 256 {
			c.route = pattern
		}
	}
}
func WithFilter(f func(*fasthttp.RequestCtx) bool) Option { return func(c *config) { c.filter = f } }
func WithServerName(name string) Option {
	return func(c *config) { httptelemetry.WithServerName(name)(&c.common) }
}
func WithTracing(v bool) Option { return func(c *config) { httptelemetry.WithTracing(v)(&c.common) } }
func WithMetrics(v bool) Option { return func(c *config) { httptelemetry.WithMetrics(v)(&c.common) } }
func WithPropagator(p propagation.TextMapPropagator) Option {
	return func(c *config) { httptelemetry.WithPropagator(p)(&c.common) }
}
func WithTraceAttributes(a ...attribute.KeyValue) Option {
	o := httptelemetry.WithTraceAttributes(a...)
	return func(c *config) { o(&c.common) }
}
func WithMetricAttributes(a ...attribute.KeyValue) Option {
	o := httptelemetry.WithMetricAttributes(a...)
	return func(c *config) { o(&c.common) }
}
func WithSpanStartOptions(a ...trace.SpanStartOption) Option {
	o := httptelemetry.WithSpanStartOptions(a...)
	return func(c *config) { o(&c.common) }
}

type contextKey struct{}

// Context returns the active request context during the handler. Copy values
// needed by background work; never retain the pooled RequestCtx itself.
func Context(ctx *fasthttp.RequestCtx) context.Context {
	if value, ok := ctx.UserValue(contextKey{}).(context.Context); ok {
		return value
	}
	return context.Background()
}

// NewHandler returns construction errors rather than reporting them globally.
func NewHandler(provider goslogx.TelemetryProvider, next fasthttp.RequestHandler, opts ...Option) (fasthttp.RequestHandler, error) {
	if next == nil {
		return nil, fmt.Errorf("fasthttp: nil handler")
	}
	c := config{}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	p := httptelemetry.Resolve(provider, ScopeName, c.common)
	metrics, err := httptelemetry.NewServerMetrics(p, c.common)
	if err != nil {
		return nil, err
	}
	tracer := p.TP.Tracer(ScopeName)
	return func(req *fasthttp.RequestCtx) {
		if c.filter != nil && !c.filter(req) {
			next(req)
			return
		}
		ctx := p.Prop.Extract(Context(req), headerCarrier{&req.Request.Header})
		method := httptelemetry.Method(string(req.Method()))
		attrs := []attribute.KeyValue{attribute.String("http.request.method", method), attribute.String("network.protocol.name", "http"), attribute.String("network.protocol.version", "1.1")}
		if c.route != "" {
			attrs = append(attrs, attribute.String("http.route", c.route))
		}
		ctx, span := tracer.Start(ctx, httptelemetry.Name(method, c.route), trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attrs...))
		finish := metrics.Begin(ctx, method)
		previous := req.UserValue(contextKey{})
		req.SetUserValue(contextKey{}, ctx)
		defer func() {
			if previous == nil {
				req.RemoveUserValue(contextKey{})
			} else {
				req.SetUserValue(contextKey{}, previous)
			}
			status := req.Response.StatusCode()
			failed := false
			if r := recover(); r != nil {
				failed = true
				span.SetStatus(codes.Error, "")
				span.SetAttributes(attribute.String("error.type", "http_error"))
				finish(c.route, 500, failed)
				span.End()
				panic(r)
			}
			span.SetAttributes(attribute.Int("http.response.status_code", status))
			if status >= 500 || ctx.Err() != nil {
				failed = true
				span.SetStatus(codes.Error, "")
				span.SetAttributes(attribute.String("error.type", "http_error"))
			}
			finish(c.route, status, failed)
			span.End()
		}()
		next(req)
	}, nil
}

type headerCarrier struct{ header *fasthttp.RequestHeader }

func (c headerCarrier) Get(key string) string { return string(c.header.Peek(key)) }
func (c headerCarrier) Set(key, value string) { c.header.Set(key, value) }
func (c headerCarrier) Keys() []string {
	keys := []string{}
	c.header.VisitAll(func(k, v []byte) { keys = append(keys, string(k)) })
	return keys
}

var WithRequestFilter = WithFilter
