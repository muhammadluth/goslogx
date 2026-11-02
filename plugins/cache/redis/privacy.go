package redis

import (
	"context"
	"github.com/muhammadluth/goslogx/internal/integration/privacy"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type privacyPolicy struct {
	statement        bool
	commandSpanNames bool
	caller           bool
	dbSystem         string
	custom           map[attribute.Key]struct{}
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

// Native redisotel names contain operations, not command arguments.
func safeName(name string, exposeOperation bool) string {
	if exposeOperation || name == "redis.dial" {
		return name
	}
	if strings.HasPrefix(name, "redis.pipeline ") {
		return "redis.pipeline"
	}
	return "redis.command"
}

func safeOperation(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "redis.pipeline ") {
		return "PIPELINE"
	}
	if name == "redis.dial" {
		return "DIAL"
	}
	if name == "" || len(name) > 64 {
		return ""
	}
	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != ' ' && char != '-' && char != '_' && char != '.' {
			return ""
		}
	}
	operation := strings.ToUpper(strings.Join(strings.Fields(name), " "))
	return operation
}

func safeAttributes(attrs []attribute.KeyValue, policy privacyPolicy) []attribute.KeyValue {
	result := []attribute.KeyValue{attribute.String("db.system", policy.dbSystem)}
	for _, attr := range attrs {
		key := string(attr.Key)
		_, custom := policy.custom[attr.Key]
		if key == "db.connection_string" {
			if endpoint := privacy.ConnectionString(attr.Value.AsString()); endpoint != "" {
				result = append(result, attribute.String(key, endpoint))
			}
			continue
		}
		if custom || key == "db.system.name" || key == "server.address" || key == "server.port" || (key == "db.redis.num_cmd" && attr.Value.Type() == attribute.INT64 && attr.Value.AsInt64() >= 0) ||
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
	attrs := safeAttributes(c.Attributes(), t.policy)
	if t.policy.commandSpanNames {
		if operation := safeOperation(name); operation != "" {
			attrs = append(attrs, attribute.String("db.operation.name", operation))
		}
	}
	clean := []trace.SpanStartOption{
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(c.SpanKind()),
		trace.WithTimestamp(c.Timestamp()),
	}
	if c.NewRoot() {
		clean = append(clean, trace.WithNewRoot())
	}
	for _, link := range c.Links() {
		clean = append(clean, trace.WithLinks(privacy.Link(link)))
	}
	ctx, span := t.Tracer.Start(ctx, safeName(name, t.policy.commandSpanNames), clean...)
	wrapped := privateSpan{Span: span, provider: t.provider, policy: t.policy}
	return trace.ContextWithSpan(ctx, wrapped), wrapped
}

type privateSpan struct {
	trace.Span
	provider trace.TracerProvider
	policy   privacyPolicy
}

func (s privateSpan) TracerProvider() trace.TracerProvider { return s.provider }
func (s privateSpan) SetName(name string) {
	s.Span.SetName(safeName(name, s.policy.commandSpanNames))
	if s.policy.commandSpanNames {
		if operation := safeOperation(name); operation != "" {
			s.Span.SetAttributes(attribute.String("db.operation.name", operation))
		}
	}
}
func (s privateSpan) SetAttributes(attrs ...attribute.KeyValue) {
	s.Span.SetAttributes(safeAttributes(attrs, s.policy)...)
}
func (s privateSpan) SetStatus(code codes.Code, description string) {
	s.Span.SetStatus(code, privacy.StatusDescription(code, description))
}
func (s privateSpan) RecordError(err error, opts ...trace.EventOption) {
	privacy.RecordError(s.Span, err, opts...)
}
func (s privateSpan) AddEvent(name string, opts ...trace.EventOption) {
	privacy.Event(s.Span, name, opts...)
}
func (s privateSpan) AddLink(link trace.Link) {
	s.Span.AddLink(privacy.Link(link))
}

func (s privateSpan) End(opts ...trace.SpanEndOption) { privacy.End(s.Span, recover(), opts...) }
