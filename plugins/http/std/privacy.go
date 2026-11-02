package std

import (
	"context"
	"github.com/muhammadluth/goslogx/internal/integration/privacy"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const configuredMetricPrefix = "goslogx.configured."

func safeAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	output := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		switch a.Key {
		case "http.route":
			if a.Value.Type() == attribute.STRING && a.Value.AsString() != "" && len(a.Value.AsString()) <= 256 {
				output = append(output, a)
			}
		case "error.type":
			if a.Value.Type() != attribute.STRING || a.Value.AsString() == "" {
				continue
			}
			a = attribute.String("error.type", privacy.ErrorLabel(a.Value.AsString(), "http_error"))
			output = append(output, a)
			continue
		case "http.request.method", "http.method":
			if a.Value.Type() == attribute.STRING {
				switch a.Value.AsString() {
				case "GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH", "_OTHER":
					output = append(output, a)
				}
			}
		case "http.response.status_code", "http.status_code":
			if a.Value.Type() == attribute.INT64 && a.Value.AsInt64() >= 100 && a.Value.AsInt64() <= 599 {
				output = append(output, a)
			}
		case "http.request.body.size", "http.response.body.size", "http.request_content_length", "http.response_content_length":
			if a.Value.Type() == attribute.INT64 && a.Value.AsInt64() >= 0 {
				output = append(output, a)
			}
		case "url.scheme", "http.scheme":
			if a.Value.Type() == attribute.STRING && (a.Value.AsString() == "http" || a.Value.AsString() == "https") {
				output = append(output, a)
			}
		case "network.protocol.name":
			if a.Value.Type() == attribute.STRING && a.Value.AsString() == "http" {
				output = append(output, a)
			}
		case "network.protocol.version", "http.flavor":
			if a.Value.Type() == attribute.STRING {
				switch a.Value.AsString() {
				case "1.0", "1.1", "2", "3":
					output = append(output, a)
				}
			}
		}
	}
	return output
}

// Body-size measurements belong in histograms, not metric dimensions.
func safeMetricAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	attributes := safeAttributes(input)
	result := attributes[:0]
	for _, a := range attributes {
		if a.Key != "http.request.body.size" && a.Key != "http.response.body.size" && a.Key != "http.request_content_length" && a.Key != "http.response_content_length" {
			result = append(result, a)
		}
	}
	return result
}

func safeConfiguredMetricAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	result := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		switch a.Key {
		case "app.channel", "app.region", "app.tier", "service.version", "deployment.environment.name":
			if a.Value.Type() == attribute.STRING && len(a.Value.AsString()) <= 128 {
				result = append(result, a)
			}
		}
	}
	return result
}

func encodeConfiguredMetricAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	safe := safeConfiguredMetricAttributes(input)
	result := make([]attribute.KeyValue, 0, len(safe))
	for _, a := range safe {
		result = append(result, attribute.KeyValue{Key: attribute.Key(configuredMetricPrefix + string(a.Key)), Value: a.Value})
	}
	return result
}

func decodeConfiguredMetricAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	result := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		key := string(a.Key)
		if !strings.HasPrefix(key, configuredMetricPrefix) {
			continue
		}
		result = append(result, attribute.KeyValue{Key: attribute.Key(strings.TrimPrefix(key, configuredMetricPrefix)), Value: a.Value})
	}
	return safeConfiguredMetricAttributes(result)
}

type privateTracerProvider struct {
	trace.TracerProvider
	route         string
	attributes    []attribute.KeyValue
	messageEvents bool
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
	clean := []trace.SpanStartOption{
		trace.WithAttributes(safeAttributes(c.Attributes())...),
		trace.WithSpanKind(c.SpanKind()),
		trace.WithTimestamp(c.Timestamp()),
	}
	if t.provider.route != "" {
		clean = append(clean, trace.WithAttributes(attribute.String("http.route", t.provider.route)))
	}
	if c.NewRoot() {
		clean = append(clean, trace.WithNewRoot())
	}
	for _, link := range c.Links() {
		clean = append(clean, trace.WithLinks(privacy.Link(link)))
	}
	clean = append(clean, trace.WithAttributes(t.provider.attributes...))
	ctx, span := t.Tracer.Start(ctx, name, clean...)
	wrapped := privateSpan{Span: span, provider: t.provider}
	return trace.ContextWithSpan(ctx, wrapped), wrapped
}

type privateSpan struct {
	trace.Span
	provider privateTracerProvider
}

func (s privateSpan) TracerProvider() trace.TracerProvider { return s.provider }
func (s privateSpan) SetAttributes(attributes ...attribute.KeyValue) {
	s.Span.SetAttributes(safeAttributes(attributes)...)
}
func (s privateSpan) SetStatus(code codes.Code, description string) {
	s.Span.SetStatus(code, privacy.StatusDescription(code, description))
}
func (s privateSpan) RecordError(err error, opts ...trace.EventOption) {
	privacy.RecordError(s.Span, err, opts...)
}
func (s privateSpan) AddEvent(name string, opts ...trace.EventOption) {
	if (name == "read" || name == "write") && !s.provider.messageEvents {
		return
	}
	privacy.Event(s.Span, name, opts...)
}
func (s privateSpan) AddLink(link trace.Link) {
	s.Span.AddLink(privacy.Link(link))
}

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
	return privateInt64Histogram{Int64Histogram: h, attributes: m.attributes}, nil
}

func (m privateMeter) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	h, err := m.Meter.Float64Histogram(name, opts...)
	if err != nil {
		return h, err
	}
	return privateFloat64Histogram{Float64Histogram: h, attributes: m.attributes}, nil
}

func recordOptions(opts []metric.RecordOption, attributes []attribute.KeyValue) []metric.RecordOption {
	c := metric.NewRecordConfig(opts)
	set := c.Attributes()
	clean := append(safeMetricAttributes(set.ToSlice()), decodeConfiguredMetricAttributes(set.ToSlice())...)
	clean = append(clean, attributes...)
	return []metric.RecordOption{metric.WithAttributes(clean...)}
}

type privateInt64Histogram struct {
	metric.Int64Histogram
	attributes []attribute.KeyValue
}

func (h privateInt64Histogram) Record(ctx context.Context, value int64, opts ...metric.RecordOption) {
	h.Int64Histogram.Record(ctx, value, recordOptions(opts, h.attributes)...)
}

type privateFloat64Histogram struct {
	metric.Float64Histogram
	attributes []attribute.KeyValue
}

func (h privateFloat64Histogram) Record(ctx context.Context, value float64, opts ...metric.RecordOption) {
	h.Float64Histogram.Record(ctx, value, recordOptions(opts, h.attributes)...)
}

func (s privateSpan) End(opts ...trace.SpanEndOption) { privacy.End(s.Span, recover(), opts...) }
