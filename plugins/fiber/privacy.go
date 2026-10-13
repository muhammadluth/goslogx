package fiber

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

func method(s string) string {
	switch s {
	case "GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH":
		return s
	}
	return "_OTHER"
}

func safeAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	output := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		switch a.Key {
		case "error.type":
			if a.Value.Type() != attribute.STRING || a.Value.AsString() == "" {
				continue
			}
			a = attribute.String("error.type", "http_error")
			output = append(output, a)
			continue
		case "http.request.method":
			a = attribute.String(string(a.Key), method(a.Value.AsString()))
		case "http.route":
			if a.Value.Type() != attribute.STRING {
				continue
			}
			a = attribute.String(string(a.Key), strings.Clone(a.Value.AsString()))
		case "http.response.status_code":
			if a.Value.Type() != attribute.INT64 || a.Value.AsInt64() < 100 || a.Value.AsInt64() > 599 {
				continue
			}
		case "http.request.body.size", "http.response.body.size":
			if a.Value.Type() != attribute.INT64 || a.Value.AsInt64() < 0 {
				continue
			}
		case "url.scheme":
			if a.Value.AsString() != "http" && a.Value.AsString() != "https" {
				continue
			}
		case "network.protocol.name":
			if a.Value.AsString() != "http" {
				continue
			}
		case "network.protocol.version":
			if a.Value.AsString() != "1.0" && a.Value.AsString() != "1.1" {
				continue
			}
		default:
			continue
		}
		output = append(output, a)
	}
	return output
}

// Body-size measurements belong in histograms, not metric dimensions.
func safeMetricAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	attributes := safeAttributes(input)
	result := attributes[:0]
	for _, a := range attributes {
		if a.Key != "http.request.body.size" && a.Key != "http.response.body.size" {
			result = append(result, a)
		}
	}
	return result
}

type privateTracerProvider struct {
	trace.TracerProvider
	serverName string
}

func (p privateTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return privateTracer{Tracer: p.TracerProvider.Tracer(ScopeName, opts...), provider: p}
}

type privateTracer struct {
	trace.Tracer
	provider privateTracerProvider
}

func (t privateTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	c := trace.NewSpanStartConfig(opts...)
	attrs := safeAttributes(c.Attributes())
	if t.provider.serverName != "" {
		attrs = append(attrs, attribute.String("server.address", t.provider.serverName))
	}
	ctx, span := t.Tracer.Start(ctx, method(name), trace.WithAttributes(attrs...), trace.WithSpanKind(c.SpanKind()), trace.WithTimestamp(c.Timestamp()))
	wrapped := privateSpan{Span: span, provider: t.provider}
	return trace.ContextWithSpan(ctx, wrapped), wrapped
}

type privateSpan struct {
	trace.Span
	provider trace.TracerProvider
}

func (s privateSpan) TracerProvider() trace.TracerProvider { return s.provider }
func (s privateSpan) SetAttributes(attrs ...attribute.KeyValue) {
	s.Span.SetAttributes(safeAttributes(attrs)...)
}
func (s privateSpan) SetStatus(code codes.Code, _ string)     { s.Span.SetStatus(code, "") }
func (s privateSpan) RecordError(error, ...trace.EventOption) {}
func (s privateSpan) AddEvent(string, ...trace.EventOption)   {}
func (s privateSpan) AddLink(trace.Link)                      {}
func (s privateSpan) End(...trace.SpanEndOption)              { s.Span.End() }

type privateMeterProvider struct {
	metric.MeterProvider
	attributes []attribute.KeyValue
}

func (p privateMeterProvider) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return privateMeter{Meter: p.MeterProvider.Meter(ScopeName, opts...), attributes: p.attributes}
}

type privateMeter struct {
	metric.Meter
	attributes []attribute.KeyValue
}

func (m privateMeter) Int64Histogram(name string, opts ...metric.Int64HistogramOption) (metric.Int64Histogram, error) {
	h, err := m.Meter.Int64Histogram(name, opts...)
	if err != nil {
		return h, err
	}
	return privateInt64Histogram{h, m.attributes}, nil
}
func (m privateMeter) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	h, err := m.Meter.Float64Histogram(name, opts...)
	if err != nil {
		return h, err
	}
	return privateFloat64Histogram{h, m.attributes}, nil
}
func (m privateMeter) Int64UpDownCounter(name string, opts ...metric.Int64UpDownCounterOption) (metric.Int64UpDownCounter, error) {
	h, err := m.Meter.Int64UpDownCounter(name, opts...)
	if err != nil {
		return h, err
	}
	return privateCounter{h, m.attributes}, nil
}
func recordOptions(opts []metric.RecordOption, attrs []attribute.KeyValue) []metric.RecordOption {
	c := metric.NewRecordConfig(opts)
	set := c.Attributes()
	return []metric.RecordOption{metric.WithAttributes(append(safeMetricAttributes(set.ToSlice()), attrs...)...)}
}

type privateInt64Histogram struct {
	metric.Int64Histogram
	attributes []attribute.KeyValue
}

func (h privateInt64Histogram) Record(ctx context.Context, v int64, opts ...metric.RecordOption) {
	h.Int64Histogram.Record(ctx, v, recordOptions(opts, h.attributes)...)
}

type privateFloat64Histogram struct {
	metric.Float64Histogram
	attributes []attribute.KeyValue
}

func (h privateFloat64Histogram) Record(ctx context.Context, v float64, opts ...metric.RecordOption) {
	h.Float64Histogram.Record(ctx, v, recordOptions(opts, h.attributes)...)
}

type privateCounter struct {
	metric.Int64UpDownCounter
	attributes []attribute.KeyValue
}

func (h privateCounter) Add(ctx context.Context, v int64, opts ...metric.AddOption) {
	c := metric.NewAddConfig(opts)
	set := c.Attributes()
	h.Int64UpDownCounter.Add(ctx, v, metric.WithAttributes(append(safeMetricAttributes(set.ToSlice()), h.attributes...)...))
}
