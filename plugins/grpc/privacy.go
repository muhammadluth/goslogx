package grpc

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

func clean(input []attribute.KeyValue, c config) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(input))
	service, method := "", ""
	for _, a := range input {
		switch a.Key {
		case "rpc.service":
			service = a.Value.AsString()
		case "rpc.method":
			method = a.Value.AsString()
		case "rpc.system", "rpc.system.name":
			if a.Value.AsString() == "grpc" {
				out = append(out, a)
			}
		case "rpc.grpc.status_code":
			if a.Value.Type() == attribute.INT64 && a.Value.AsInt64() >= 0 && a.Value.AsInt64() <= 16 {
				out = append(out, a)
			}
		case "error.type":
			out = append(out, attribute.String("error.type", "grpc_error"))
		}
	}
	if c.methods[service+"/"+method] {
		out = append(out, attribute.String("rpc.service", service), attribute.String("rpc.method", method))
	}
	return out
}

func safeMetricAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		switch a.Key {
		case "app.channel", "service.version", "deployment.environment.name":
			if a.Value.Type() == attribute.STRING && len(a.Value.AsString()) <= 128 {
				out = append(out, a)
			}
		}
	}
	return out
}

func safeMessageAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		switch a.Key {
		case "rpc.message.type":
			if a.Value.Type() == attribute.STRING && (a.Value.AsString() == "SENT" || a.Value.AsString() == "RECEIVED") {
				out = append(out, a)
			}
		case "rpc.message.id", "rpc.message.compressed_size", "rpc.message.uncompressed_size":
			if a.Value.Type() == attribute.INT64 && a.Value.AsInt64() >= 0 {
				out = append(out, a)
			}
		}
	}
	return out
}

type privateTP struct {
	trace.TracerProvider
	c config
}

func (p privateTP) Tracer(_ string, opts ...trace.TracerOption) trace.Tracer {
	return privateTracer{p.TracerProvider.Tracer(ScopeName, opts...), p}
}

type privateTracer struct {
	trace.Tracer
	p privateTP
}

func (t privateTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	c := trace.NewSpanStartConfig(opts...)
	name = strings.TrimPrefix(name, "/")
	if !t.p.c.methods[name] {
		name = "gRPC"
	}
	attrs := append(clean(c.Attributes(), t.p.c), t.p.c.traceAttrs...)
	safe := []trace.SpanStartOption{trace.WithSpanKind(c.SpanKind()), trace.WithTimestamp(c.Timestamp()), trace.WithAttributes(attrs...)}
	if c.NewRoot() {
		safe = append(safe, trace.WithNewRoot())
	}
	for _, link := range c.Links() {
		safe = append(safe, trace.WithLinks(trace.Link{SpanContext: link.SpanContext}))
	}
	ctx, s := t.Tracer.Start(ctx, name, safe...)
	wrapped := privateSpan{s, t.p}
	return trace.ContextWithSpan(ctx, wrapped), wrapped
}

type privateSpan struct {
	trace.Span
	p privateTP
}

func (s privateSpan) TracerProvider() trace.TracerProvider  { return s.p }
func (s privateSpan) SetAttributes(a ...attribute.KeyValue) { s.Span.SetAttributes(clean(a, s.p.c)...) }
func (s privateSpan) SetStatus(code codes.Code, _ string)   { s.Span.SetStatus(code, "") }
func (s privateSpan) RecordError(err error, _ ...trace.EventOption) {
	if err != nil {
		s.Span.SetAttributes(attribute.String("error.type", "grpc_error"))
	}
}
func (s privateSpan) AddEvent(name string, opts ...trace.EventOption) {
	if name != "message" || len(s.p.c.messageEvents) == 0 {
		return
	}
	c := trace.NewEventConfig(opts...)
	s.Span.AddEvent("message", trace.WithTimestamp(c.Timestamp()), trace.WithAttributes(safeMessageAttributes(c.Attributes())...))
}
func (s privateSpan) AddLink(link trace.Link) {
	s.Span.AddLink(trace.Link{SpanContext: link.SpanContext})
}

type privateMP struct {
	metric.MeterProvider
	c config
}

func (p privateMP) Meter(_ string, opts ...metric.MeterOption) metric.Meter {
	return privateMeter{p.MeterProvider.Meter(ScopeName, opts...), p.c}
}

type privateMeter struct {
	metric.Meter
	c config
}

func (m privateMeter) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	h, e := m.Meter.Float64Histogram(name, opts...)
	if e != nil {
		return h, e
	}
	return floatHistogram{h, m.c}, nil
}
func (m privateMeter) Int64Histogram(name string, opts ...metric.Int64HistogramOption) (metric.Int64Histogram, error) {
	h, e := m.Meter.Int64Histogram(name, opts...)
	if e != nil {
		return h, e
	}
	return intHistogram{h, m.c}, nil
}
func metricOptions(opts []metric.RecordOption, c config) []metric.RecordOption {
	cfg := metric.NewRecordConfig(opts)
	set := cfg.Attributes()
	a := append(clean(set.ToSlice(), c), safeMetricAttributes(set.ToSlice())...)
	a = append(a, c.metricAttrs...)
	return []metric.RecordOption{metric.WithAttributes(a...)}
}

type floatHistogram struct {
	metric.Float64Histogram
	c config
}

func (h floatHistogram) Record(ctx context.Context, value float64, opts ...metric.RecordOption) {
	h.Float64Histogram.Record(ctx, value, metricOptions(opts, h.c)...)
}

type intHistogram struct {
	metric.Int64Histogram
	c config
}

func (h intHistogram) Record(ctx context.Context, value int64, opts ...metric.RecordOption) {
	h.Int64Histogram.Record(ctx, value, metricOptions(opts, h.c)...)
}
