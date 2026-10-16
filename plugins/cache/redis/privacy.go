package redis

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func safeName(name string) string {
	if name == "redis.dial" {
		return name
	}
	if strings.HasPrefix(name, "redis.pipeline ") {
		return "redis.pipeline"
	}
	return "redis.command"
}

func safeAttributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	result := []attribute.KeyValue{attribute.String("db.system", "redis")}
	for _, attr := range attrs {
		if attr.Key == "db.redis.num_cmd" && attr.Value.Type() == attribute.INT64 && attr.Value.AsInt64() >= 0 {
			result = append(result, attr)
		}
	}
	return result
}

type privateTracerProvider struct{ trace.TracerProvider }

func (p privateTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return privateTracer{Tracer: p.TracerProvider.Tracer(ScopeName, opts...), provider: p}
}

type privateTracer struct {
	trace.Tracer
	provider trace.TracerProvider
}

func (t privateTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	c := trace.NewSpanStartConfig(opts...)
	clean := []trace.SpanStartOption{
		trace.WithAttributes(safeAttributes(c.Attributes())...),
		trace.WithSpanKind(c.SpanKind()),
		trace.WithTimestamp(c.Timestamp()),
	}
	if c.NewRoot() {
		clean = append(clean, trace.WithNewRoot())
	}
	for _, link := range c.Links() {
		clean = append(clean, trace.WithLinks(trace.Link{SpanContext: link.SpanContext}))
	}
	ctx, span := t.Tracer.Start(ctx, safeName(name), clean...)
	wrapped := privateSpan{Span: span, provider: t.provider}
	return trace.ContextWithSpan(ctx, wrapped), wrapped
}

type privateSpan struct {
	trace.Span
	provider trace.TracerProvider
}

func (s privateSpan) TracerProvider() trace.TracerProvider { return s.provider }
func (s privateSpan) SetName(name string)                  { s.Span.SetName(safeName(name)) }
func (s privateSpan) SetAttributes(attrs ...attribute.KeyValue) {
	s.Span.SetAttributes(safeAttributes(attrs)...)
}
func (s privateSpan) SetStatus(code codes.Code, _ string)     { s.Span.SetStatus(code, "") }
func (s privateSpan) RecordError(error, ...trace.EventOption) {}
func (s privateSpan) AddEvent(string, ...trace.EventOption)   {}
func (s privateSpan) AddLink(link trace.Link) {
	s.Span.AddLink(trace.Link{SpanContext: link.SpanContext})
}
