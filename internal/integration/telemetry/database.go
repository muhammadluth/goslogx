package telemetry

import (
	"context"
	"database/sql"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"strings"
)

// SafeDBProvider prevents third-party query instrumentation from recording raw errors.
// Query text must be excluded upstream or supplied by an explicit sanitizer.
func SafeDBProvider(tp trace.TracerProvider, scope string, capture ...bool) trace.TracerProvider {
	allow := len(capture) > 0 && capture[0]
	return safeDBProvider{TracerProvider: tp, scope: scope, capture: allow}
}

type safeDBProvider struct {
	trace.TracerProvider
	scope   string
	capture bool
}

func (p safeDBProvider) Tracer(_ string, opts ...trace.TracerOption) trace.Tracer {
	return safeDBTracer{Tracer: p.TracerProvider.Tracer(p.scope, opts...), provider: p}
}

type safeDBTracer struct {
	trace.Tracer
	provider safeDBProvider
}

func (t safeDBTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	c := trace.NewSpanStartConfig(opts...)
	clean := []trace.SpanStartOption{trace.WithSpanKind(c.SpanKind()), trace.WithTimestamp(c.Timestamp()), trace.WithAttributes(dbAttributes(c.Attributes(), t.provider.capture)...)}
	if c.NewRoot() {
		clean = append(clean, trace.WithNewRoot())
	}
	for _, link := range c.Links() {
		clean = append(clean, trace.WithLinks(trace.Link{SpanContext: link.SpanContext}))
	}
	ctx, s := t.Tracer.Start(ctx, name, clean...)
	wrapped := safeDBSpan{Span: s, provider: t.provider}
	return trace.ContextWithSpan(ctx, wrapped), wrapped
}
func dbAttributes(attrs []attribute.KeyValue, capture bool) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, a := range attrs {
		switch a.Key {
		case "db.system.name", "db.namespace":
			out = append(out, a)
		case "db.operation.name":
			out = append(out, attribute.String("db.operation.name", DBOperation(a.Value.AsString())))
		case "db.query.text":
			if capture {
				out = append(out, a)
			}
		case "error.type":
			out = append(out, attribute.String("error.type", "database_error"))
		}
	}
	return out
}

type safeDBSpan struct {
	trace.Span
	provider safeDBProvider
}

func (s safeDBSpan) TracerProvider() trace.TracerProvider { return s.provider }
func (s safeDBSpan) SetAttributes(attrs ...attribute.KeyValue) {
	s.Span.SetAttributes(dbAttributes(attrs, s.provider.capture)...)
}
func (s safeDBSpan) RecordError(err error, _ ...trace.EventOption) {
	if err != nil {
		s.Span.SetAttributes(attribute.String("error.type", "database_error"))
	}
}
func (s safeDBSpan) SetStatus(code codes.Code, _ string)   { s.Span.SetStatus(code, "") }
func (s safeDBSpan) AddEvent(string, ...trace.EventOption) {}
func (s safeDBSpan) AddLink(link trace.Link) {
	s.Span.AddLink(trace.Link{SpanContext: link.SpanContext})
}

// SQLPool observes pool state through the supplied provider, without a goroutine.
func SQLPool(meter metric.Meter, db *sql.DB, namespace string) error {
	attrs := []attribute.KeyValue{attribute.String("db.client.connection.pool.name", "database")}
	if namespace != "" {
		attrs = append(attrs, attribute.String("db.namespace", namespace))
	}
	_, err := meter.Int64ObservableUpDownCounter("db.client.connection.count", metric.WithUnit("{connection}"), metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
		s := db.Stats()
		used := append(append([]attribute.KeyValue(nil), attrs...), attribute.String("db.client.connection.state", "used"))
		idle := append(append([]attribute.KeyValue(nil), attrs...), attribute.String("db.client.connection.state", "idle"))
		o.Observe(int64(s.InUse), metric.WithAttributes(used...))
		o.Observe(int64(s.Idle), metric.WithAttributes(idle...))
		return nil
	}))
	if err != nil {
		return err
	}
	_, err = meter.Int64ObservableUpDownCounter("db.client.connection.max", metric.WithUnit("{connection}"), metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
		o.Observe(int64(db.Stats().MaxOpenConnections), metric.WithAttributes(attrs...))
		return nil
	}))
	if err != nil {
		return err
	}
	return nil
}

func (s safeDBSpan) SetName(name string) {
	parts := strings.Fields(name)
	op := "QUERY"
	if len(parts) > 0 {
		op = DBOperation(parts[0])
	}
	s.Span.SetName(op)
}
