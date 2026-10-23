package redis

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type privacyPolicy struct {
	statement bool
	caller    bool
	dbSystem  string
	custom    map[attribute.Key]struct{}
}

func safeConfiguredAttributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	result := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		key := strings.ToLower(string(attr.Key))
		if key == "" || strings.Contains(key, "password") || strings.Contains(key, "credential") ||
			strings.Contains(key, "authorization") || strings.Contains(key, "token") ||
			strings.Contains(key, "secret") || strings.Contains(key, "statement") ||
			key == "db.system" || key == "pool.name" || key == "server.address" ||
			key == "server.port" || key == "db.connection_string" {
			continue
		}
		result = append(result, attr)
	}
	return result
}

func attributeKeys(attrs []attribute.KeyValue) map[attribute.Key]struct{} {
	keys := make(map[attribute.Key]struct{}, len(attrs))
	for _, attr := range attrs {
		keys[attr.Key] = struct{}{}
	}
	return keys
}

func safeName(name string) string {
	if name == "redis.dial" {
		return name
	}
	if strings.HasPrefix(name, "redis.pipeline ") {
		return "redis.pipeline"
	}
	return "redis.command"
}

func safeAttributes(attrs []attribute.KeyValue, policy privacyPolicy) []attribute.KeyValue {
	result := []attribute.KeyValue{attribute.String("db.system", policy.dbSystem)}
	for _, attr := range attrs {
		key := string(attr.Key)
		_, custom := policy.custom[attr.Key]
		if custom || (key == "db.redis.num_cmd" && attr.Value.Type() == attribute.INT64 && attr.Value.AsInt64() >= 0) ||
			(policy.statement && key == "db.statement") ||
			(policy.caller && (key == "code.function" || key == "code.filepath" || key == "code.lineno")) {
			result = append(result, attr)
		}
	}
	return result
}

type privateTracerProvider struct {
	trace.TracerProvider
	policy privacyPolicy
}

func (p privateTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return privateTracer{Tracer: p.TracerProvider.Tracer(ScopeName, opts...), provider: p, policy: p.policy}
}

type privateTracer struct {
	trace.Tracer
	provider trace.TracerProvider
	policy   privacyPolicy
}

func (t privateTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	c := trace.NewSpanStartConfig(opts...)
	clean := []trace.SpanStartOption{
		trace.WithAttributes(safeAttributes(c.Attributes(), t.policy)...),
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
	wrapped := privateSpan{Span: span, provider: t.provider, policy: t.policy}
	return trace.ContextWithSpan(ctx, wrapped), wrapped
}

type privateSpan struct {
	trace.Span
	provider trace.TracerProvider
	policy   privacyPolicy
}

func (s privateSpan) TracerProvider() trace.TracerProvider { return s.provider }
func (s privateSpan) SetName(name string)                  { s.Span.SetName(safeName(name)) }
func (s privateSpan) SetAttributes(attrs ...attribute.KeyValue) {
	s.Span.SetAttributes(safeAttributes(attrs, s.policy)...)
}
func (s privateSpan) SetStatus(code codes.Code, _ string)     { s.Span.SetStatus(code, "") }
func (s privateSpan) RecordError(error, ...trace.EventOption) {}
func (s privateSpan) AddEvent(string, ...trace.EventOption)   {}
func (s privateSpan) AddLink(link trace.Link) {
	s.Span.AddLink(trace.Link{SpanContext: link.SpanContext})
}
