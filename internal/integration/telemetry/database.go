package telemetry

import (
	"context"
	"database/sql"
	"github.com/muhammadluth/goslogx/internal/integration/privacy"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
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
	config  *Config
	policy  DBPolicy
}

// ConfiguredDBProvider applies statement and custom-span policies without
// changing the upstream operation names or its semantic convention version.
// DBPolicy enables narrowly scoped diagnostic fields for a particular adapter.
// ExtraAttributes must validate and bound every value it returns.
type DBPolicy struct {
	ExtraAttributes func([]attribute.KeyValue) []attribute.KeyValue
}

func ConfiguredDBProvider(tp trace.TracerProvider, scope string, cfg Config, policies ...DBPolicy) trace.TracerProvider {
	p := safeDBProvider{TracerProvider: tp, scope: scope, capture: cfg.StatementCapture, config: &cfg}
	if len(policies) > 0 {
		p.policy = policies[0]
	}
	return p
}

func (p safeDBProvider) attributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	clean := dbAttributes(attrs, p.capture)
	if p.config != nil {
		for i, a := range clean {
			if a.Key == "db.statement" || a.Key == "db.query.text" {
				clean[i] = attribute.String(string(a.Key), p.config.QueryText(a.Value.AsString()))
			}
		}
	}
	if p.policy.ExtraAttributes != nil {
		clean = append(clean, p.policy.ExtraAttributes(attrs)...)
	}
	return clean
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
	clean := []trace.SpanStartOption{trace.WithSpanKind(c.SpanKind()), trace.WithTimestamp(c.Timestamp()), trace.WithAttributes(t.provider.attributes(c.Attributes())...)}
	if t.provider.config != nil {
		clean = append(clean, trace.WithAttributes(t.provider.config.TraceAttributes...))
	}
	if c.NewRoot() {
		clean = append(clean, trace.WithNewRoot())
	}
	for _, link := range c.Links() {
		clean = append(clean, trace.WithLinks(privacy.Link(link)))
	}
	ctx, s := t.Tracer.Start(ctx, name, clean...)
	wrapped := safeDBSpan{Span: s, provider: t.provider}
	return trace.ContextWithSpan(ctx, wrapped), wrapped
}
func dbAttributes(attrs []attribute.KeyValue, capture bool) []attribute.KeyValue {
	out := privacy.Attributes(attrs)
	for _, a := range attrs {
		switch a.Key {
		case "network.peer.address", "network.peer.port", "network.transport", "pgx.rows_affected", "pgx.sql_state", "pgx.operation.type", "pgx.prepare_stmt.name":
			out = append(out, a)
		case "db.query.text", "db.statement":
			if capture {
				out = append(out, a)
			}
		case "db.connection_string":
			if endpoint := privacy.ConnectionString(a.Value.AsString()); endpoint != "" {
				out = append(out, attribute.String(string(a.Key), endpoint))
			}
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
	s.Span.SetAttributes(s.provider.attributes(attrs)...)
}
func (s safeDBSpan) RecordError(err error, opts ...trace.EventOption) {
	privacy.RecordError(s.Span, err, opts...)
}
func (s safeDBSpan) SetStatus(code codes.Code, description string) {
	s.Span.SetStatus(code, privacy.StatusDescription(code, description))
}
func (s safeDBSpan) AddEvent(name string, opts ...trace.EventOption) {
	privacy.Event(s.Span, name, opts...)
}
func (s safeDBSpan) AddLink(link trace.Link) {
	s.Span.AddLink(privacy.Link(link))
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
	s.Span.SetName(name)
}

func (s safeDBSpan) End(opts ...trace.SpanEndOption) { privacy.End(s.Span, recover(), opts...) }
