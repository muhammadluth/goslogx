package bun

import (
	"context"
	"errors"
	"github.com/muhammadluth/goslogx/internal/integration/privacy"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Keep the goslogx scope while allowing bunotel to own its instrumentation.
type scopedMeterProvider struct {
	metric.MeterProvider
	attributes []attribute.KeyValue
	initErr    *error
}

func (p scopedMeterProvider) Meter(_ string, opts ...metric.MeterOption) metric.Meter {
	return privateMeter{Meter: p.MeterProvider.Meter(instrumentationScope, opts...), attributes: p.attributes, initErr: p.initErr}
}

type privateMeter struct {
	metric.Meter
	attributes []attribute.KeyValue
	initErr    *error
}

func (m privateMeter) Int64Histogram(name string, opts ...metric.Int64HistogramOption) (metric.Int64Histogram, error) {
	histogram, err := m.Meter.Int64Histogram(name, opts...)
	if err != nil {
		if m.initErr != nil {
			*m.initErr = errors.Join(*m.initErr, err)
		}
		return nil, err
	}
	return privateInt64Histogram{Int64Histogram: histogram, attributes: m.attributes}, nil
}

type privateInt64Histogram struct {
	metric.Int64Histogram
	attributes []attribute.KeyValue
}

func (h privateInt64Histogram) Record(ctx context.Context, value int64, opts ...metric.RecordOption) {
	set := metric.NewRecordConfig(opts).Attributes()
	attrs := set.ToSlice()
	clean := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		switch attr.Key {
		case "db.operation", "db.operation.name":
			clean = append(clean, attribute.String(string(attr.Key), safeOperation(attr.Value.AsString())))
		case "db.system", "db.system.name", "db.name", "db.namespace":
			clean = append(clean, attr)
		}
	}
	h.Int64Histogram.Record(ctx, value, metric.WithAttributes(append(clean, h.attributes...)...))
}

type privateTracerProvider struct {
	trace.TracerProvider
	config config
}

func (p privateTracerProvider) Tracer(_ string, opts ...trace.TracerOption) trace.Tracer {
	return privateTracer{Tracer: p.TracerProvider.Tracer(instrumentationScope, opts...), provider: p}
}

type privateTracer struct {
	trace.Tracer
	provider privateTracerProvider
}

func (t privateTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	c := trace.NewSpanStartConfig(opts...)
	clean := []trace.SpanStartOption{
		trace.WithSpanKind(c.SpanKind()),
		trace.WithAttributes(privacy.Attributes(c.Attributes())...),
		trace.WithTimestamp(c.Timestamp()),
	}
	if c.NewRoot() {
		clean = append(clean, trace.WithNewRoot())
	}
	for _, link := range c.Links() {
		clean = append(clean, trace.WithLinks(privacy.Link(link)))
	}
	clean = append(clean, trace.WithAttributes(t.provider.config.policy.TraceAttributes...))
	ctx, span := t.Tracer.Start(ctx, name, clean...)
	wrapped := privateSpan{Span: span, provider: t.provider}
	return trace.ContextWithSpan(ctx, wrapped), wrapped
}

type privateSpan struct {
	trace.Span
	provider privateTracerProvider
}

func (s privateSpan) TracerProvider() trace.TracerProvider { return s.provider }
func (s privateSpan) SetName(name string)                  { s.Span.SetName(name) }
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
func (s privateSpan) SetAttributes(attrs ...attribute.KeyValue) {
	clean := privacy.Attributes(attrs)
	for i, a := range clean {
		if a.Key == "db.operation" || a.Key == "db.operation.name" {
			clean[i] = attribute.String(string(a.Key), safeOperation(a.Value.AsString()))
		}
	}
	for _, a := range attrs {
		switch a.Key {
		case "db.statement", "db.query.text":
			if s.provider.config.policy.StatementCapture {
				clean = append(clean, attribute.String(string(a.Key), s.provider.config.policy.QueryText(a.Value.AsString())))
			}
		case "code.function", "code.filepath", "code.lineno":
			if s.provider.config.caller {
				clean = append(clean, a)
			}
		}
	}
	s.Span.SetAttributes(clean...)
}
