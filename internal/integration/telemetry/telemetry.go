// Package telemetry supplies shared, provider-explicit policies to integrations.
package telemetry

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/privacy"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

type Option func(*Config)
type Config struct {
	optionTarget                         any
	Normalizer                           func(string) string
	Namespace, Consumer, Group, ClientID string
	RoutingNormalizer                    func(string) string
	StatementCapture                     bool
	RawStatement                         bool
	ConsumerLinks                        bool
	StatementSanitizer                   func(string) string
	Propagator                           propagation.TextMapPropagator
	TraceAttributes, MetricAttributes    []attribute.KeyValue
	TracingDisabled, MetricsDisabled     bool
	OnError                              func(error)
}

func WithDestinationNormalizer(f func(string) string) Option {
	return func(c *Config) { c.Normalizer = f }
}
func WithDBName(name string) Option        { return func(c *Config) { c.Namespace = static(name) } }
func WithConsumerName(name string) Option  { return func(c *Config) { c.Consumer = static(name) } }
func WithConsumerGroup(name string) Option { return func(c *Config) { c.Group = static(name) } }
func WithClientID(name string) Option      { return func(c *Config) { c.ClientID = static(name) } }
func WithStatementCapture(enabled bool) Option {
	return func(c *Config) { c.StatementCapture = enabled }
}
func WithStatementSanitizer(f func(string) string) Option {
	return func(c *Config) { c.StatementSanitizer = f }
}

// WithRawStatement explicitly permits sensitive query text in trusted diagnostics.
func WithRawStatement(enabled bool) Option {
	return func(c *Config) { c.RawStatement = enabled; c.StatementCapture = enabled }
}
func WithPropagator(p propagation.TextMapPropagator) Option {
	return func(c *Config) { c.Propagator = p }
}

// WithConsumerLinks requests links instead of parentage in native consumers
// whose upstream instrumentation supports this relationship.
func WithConsumerLinks(enabled bool) Option { return func(c *Config) { c.ConsumerLinks = enabled } }
func WithTraceAttributes(attrs ...attribute.KeyValue) Option {
	copy := privacy.Attributes(attrs)
	return func(c *Config) { c.TraceAttributes = append(c.TraceAttributes, copy...) }
}

// WithMetricAttributes accepts static bounded deployment labels, never request IDs.
func WithMetricAttributes(attrs ...attribute.KeyValue) Option {
	var safe []attribute.KeyValue
	for _, a := range attrs {
		switch a.Key {
		case "app.channel", "app.region", "app.tier", "service.version", "deployment.environment.name":
			if a.Value.Type() == attribute.STRING && len(a.Value.AsString()) <= 128 {
				safe = append(safe, a)
			}
		}
	}
	return func(c *Config) { c.MetricAttributes = append(c.MetricAttributes, safe...) }
}
func WithTracing(enabled bool) Option       { return func(c *Config) { c.TracingDisabled = !enabled } }
func WithMetrics(enabled bool) Option       { return func(c *Config) { c.MetricsDisabled = !enabled } }
func WithErrorHandler(f func(error)) Option { return func(c *Config) { c.OnError = f } }
func static(s string) string {
	if len(s) > 128 {
		return ""
	}
	return strings.Clone(s)
}
func Nil(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return r.IsNil()
	}
	return false
}

// Resolve never consults global providers; disabled or missing signals are no-ops.
func Resolve(provider goslogx.TelemetryProvider, cfg Config) (trace.TracerProvider, metric.MeterProvider, propagation.TextMapPropagator) {
	var tp trace.TracerProvider = tracenoop.NewTracerProvider()
	var mp metric.MeterProvider = metricnoop.NewMeterProvider()
	var prop propagation.TextMapPropagator = propagation.TraceContext{}
	if !Nil(provider) {
		if p := provider.TracerProvider(); !Nil(p) && !cfg.TracingDisabled {
			tp = p
		}
		if p := provider.MeterProvider(); !Nil(p) && !cfg.MetricsDisabled {
			mp = p
		}
		if p := provider.TextMapPropagator(); !Nil(p) {
			prop = p
		}
	}
	if !Nil(cfg.Propagator) {
		prop = cfg.Propagator
	}
	return tp, mp, prop
}

type Instrumentation struct {
	Config               Config
	Tracer               trace.Tracer
	Meter                metric.Meter
	Propagator           propagation.TextMapPropagator
	system               string
	messaging            bool
	duration, processing metric.Float64Histogram
	sent, consumed       metric.Int64Counter
	mu                   sync.Mutex
	destinations         map[string]struct{}
}

func New(provider goslogx.TelemetryProvider, scope, system string, messaging bool, opts ...Option) (*Instrumentation, error) {
	cfg := Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.StatementCapture && cfg.StatementSanitizer == nil && !cfg.RawStatement {
		return nil, errors.New("instrumentation: statement capture requires an explicit sanitizer")
	}
	tp, mp, prop := Resolve(provider, cfg)
	t := &Instrumentation{Config: cfg, Tracer: tp.Tracer(scope), Meter: mp.Meter(scope), Propagator: prop, system: system, messaging: messaging, destinations: make(map[string]struct{})}
	name := "db.client.operation.duration"
	if messaging {
		name = "messaging.client.operation.duration"
	}
	var err error
	t.duration, err = t.Meter.Float64Histogram(name, metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	if messaging {
		t.processing, err = t.Meter.Float64Histogram("messaging.process.duration", metric.WithUnit("s"))
		if err != nil {
			return nil, err
		}
		t.sent, err = t.Meter.Int64Counter("messaging.client.sent.messages", metric.WithUnit("{message}"))
		if err != nil {
			return nil, err
		}
		t.consumed, err = t.Meter.Int64Counter("messaging.client.consumed.messages", metric.WithUnit("{message}"))
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}

// Destination is opt-in and capped to 64 distinct normalized values per instance.
// Unconfigured instrumentation omits potentially dynamic destination names.
func (t *Instrumentation) Destination(raw string) string {
	if t.Config.Normalizer == nil {
		return ""
	}
	name := static(t.Config.Normalizer(raw))
	if name == "" {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.destinations[name]; ok {
		return name
	}
	if len(t.destinations) >= 64 {
		return "_OTHER"
	}
	t.destinations[name] = struct{}{}
	return name
}
func (t *Instrumentation) attributes(op, destination string) []attribute.KeyValue {
	if !t.messaging {
		a := []attribute.KeyValue{attribute.String("db.system.name", t.system), attribute.String("db.operation.name", op)}
		if t.Config.Namespace != "" {
			a = append(a, attribute.String("db.namespace", t.Config.Namespace))
		}
		return a
	}
	a := []attribute.KeyValue{attribute.String("messaging.system", t.system), attribute.String("messaging.operation.name", op), attribute.String("messaging.operation.type", operationType(op))}
	if destination != "" {
		a = append(a, attribute.String("messaging.destination.name", destination))
	}
	if t.Config.Consumer != "" {
		a = append(a, attribute.String("messaging.consumer.name", t.Config.Consumer))
	}
	if t.Config.Group != "" {
		a = append(a, attribute.String("messaging.consumer.group.name", t.Config.Group))
	}
	if t.Config.ClientID != "" {
		a = append(a, attribute.String("messaging.client.id", t.Config.ClientID))
	}
	return a
}
func operationType(op string) string {
	switch op {
	case "send", "publish", "request":
		return "send"
	case "receive", "fetch", "consume":
		return "receive"
	case "process":
		return "process"
	case "ack", "nack", "reject":
		return "settle"
	}
	return "_OTHER"
}

// Begin records safe metadata only. Finish is idempotent, including asynchronous callbacks.
func (t *Instrumentation) Begin(ctx context.Context, op, destination string, kind trace.SpanKind) (context.Context, func(error)) {
	return t.BeginCount(ctx, op, destination, kind, 1)
}
func (t *Instrumentation) BeginCount(ctx context.Context, op, destination string, kind trace.SpanKind, count int64) (context.Context, func(error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	if t.messaging {
		switch op {
		case "send", "publish", "request", "receive", "fetch", "consume", "process", "ack", "nack", "reject":
		default:
			op = "_OTHER"
		}
	} else {
		op = DBOperation(op)
	}
	dest := ""
	if t.messaging {
		dest = t.Destination(destination)
	}
	attrs := t.attributes(op, dest)
	name := op
	if t.messaging {
		name = t.system + " " + op
		if dest != "" {
			name = dest + " " + op
		}
	}
	spanAttrs := append(append([]attribute.KeyValue(nil), t.Config.TraceAttributes...), attrs...)
	attrs = append(attrs, t.Config.MetricAttributes...)
	ctx, span := t.Tracer.Start(ctx, name, trace.WithSpanKind(kind), trace.WithAttributes(spanAttrs...))
	started := time.Now()
	var once sync.Once
	return ctx, func(err error) {
		once.Do(func() {
			defer span.End()
			recorded := attrs
			if err != nil {
				errorType := privacy.ErrorType(err)
				a := attribute.String("error.type", errorType)
				span.SetAttributes(a)
				privacy.RecordError(span, err)
				span.SetStatus(codes.Error, privacy.StatusDescription(codes.Error, ""))
				recorded = append(append([]attribute.KeyValue(nil), attrs...), a)
			}
			hist := t.duration
			if t.messaging && op == "process" {
				hist = t.processing
			}
			hist.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(recorded...))
			if t.messaging && err == nil && count > 0 {
				switch operationType(op) {
				case "send":
					t.sent.Add(ctx, count, metric.WithAttributes(attrs...))
				case "receive":
					t.consumed.Add(ctx, count, metric.WithAttributes(attrs...))
				}
			}
		})
	}
}

// Process extracts the configured propagator and measures handler execution.
// It neither acknowledges messages nor changes callback errors.
func (t *Instrumentation) Process(ctx context.Context, carrier propagation.TextMapCarrier, destination string, fn func(context.Context) error) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if carrier != nil {
		ctx = t.Propagator.Extract(ctx, carrier)
	}
	ctx, finish := t.BeginCount(ctx, "process", destination, trace.SpanKindConsumer, 0)
	// Delivery is observable at callback entry; it does not require inventing a
	// zero-duration receive span, and it is counted even when processing fails.
	// consumed is only allocated for messaging instrumentations; guard defensively
	// so a database Instrumentation cannot panic if Process is ever misused.
	attrs := append(t.attributes("receive", t.Destination(destination)), t.Config.MetricAttributes...)
	if t.consumed != nil {
		t.consumed.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	defer func() {
		if r := recover(); r != nil {
			finish(errors.New("message handler panic"))
			panic(r)
		}
		finish(err)
	}()
	return fn(ctx)
}
func (t *Instrumentation) Statement(ctx context.Context, statement string) {
	if !t.Config.StatementCapture {
		return
	}
	sanitized := t.Config.QueryText(statement)
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("db.query.text", sanitized))
}

// QueryText applies the explicit capture policy without ever making text a metric label.
func (c Config) QueryText(statement string) string {
	if !c.StatementCapture {
		return ""
	}
	if c.StatementSanitizer != nil {
		statement = c.StatementSanitizer(statement)
	} else if !c.RawStatement {
		return ""
	}
	if len(statement) > 4096 {
		statement = statement[:4096]
	}
	return statement
}
func (t *Instrumentation) Report(err error) {
	if err != nil && t.Config.OnError != nil {
		t.Config.OnError(err)
	}
}
func DBOperation(op string) string {
	switch strings.ToUpper(op) {
	case "MATCH", "OPTIONAL", "MERGE", "RETURN", "CALL", "UNWIND", "WITH", "LOAD", "PROFILE", "EXPLAIN", "SET", "DETACH", "REMOVE", "SHOW", "SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "ALTER", "DROP", "TRUNCATE", "BEGIN", "COMMIT", "ROLLBACK", "QUERY", "EXEC", "CONNECT", "PING", "BATCH", "COPY", "FIND", "INSERTONE", "INSERTMANY", "UPDATEONE", "UPDATEMANY", "DELETEONE", "DELETEMANY", "AGGREGATE", "GETITEM", "PUTITEM", "UPDATEITEM", "DELETEITEM", "SCAN", "BATCHGETITEM", "BATCHWRITEITEM", "TRANSACTGETITEMS", "TRANSACTWRITEITEMS", "SEARCH", "INDEX", "BULK", "RUN", "NEXT", "CLOSE":
		return strings.ToUpper(op)
	}
	return "QUERY"
}

// Run keeps lifecycle/error handling consistent for callback-based adapters.
func (t *Instrumentation) Run(ctx context.Context, op string, kind trace.SpanKind, fn func(context.Context) error) (err error) {
	ctx, finish := t.Begin(ctx, op, "", kind)
	defer func() {
		if r := recover(); r != nil {
			finish(errors.New("database operation panic"))
			panic(r)
		}
		finish(err)
	}()
	return fn(ctx)
}

// Inject replaces propagation fields using OTel rather than protocol-specific formatting.
func (t *Instrumentation) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	for _, key := range t.Propagator.Fields() {
		carrier.Set(key, "")
	}
	t.Propagator.Inject(ctx, carrier)
}

// RecordDatabase records callback timings without adding another span.
func (t *Instrumentation) RecordDatabase(ctx context.Context, op string, elapsed time.Duration, err error) {
	attrs := t.attributes(DBOperation(op), "")
	if err != nil {
		attrs = append(attrs, attribute.String("error.type", privacy.ErrorType(err)))
	}
	attrs = append(attrs, t.Config.MetricAttributes...)
	t.duration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs...))
}
