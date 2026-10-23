package httptelemetry

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const configuredMetricPrefix = "goslogx.configured."

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

// EncodeMetricAttributes marks validated callback attributes so the private
// meter can distinguish them from request-controlled upstream attributes.
func EncodeMetricAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	safe := safeConfiguredMetricAttributes(input)
	result := make([]attribute.KeyValue, 0, len(safe))
	for _, a := range safe {
		result = append(result, attribute.KeyValue{Key: attribute.Key(configuredMetricPrefix + string(a.Key)), Value: a.Value})
	}
	return result
}

func decodeMetricAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	result := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		key := string(a.Key)
		if strings.HasPrefix(key, configuredMetricPrefix) {
			result = append(result, attribute.KeyValue{Key: attribute.Key(strings.TrimPrefix(key, configuredMetricPrefix)), Value: a.Value})
		}
	}
	return safeConfiguredMetricAttributes(result)
}

func safeAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	output := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		switch a.Key {
		case "http.route":
			if a.Value.Type() == attribute.STRING && len(a.Value.AsString()) <= 256 && a.Value.AsString() != "" {
				output = append(output, a)
			}
		case "error.type":
			if a.Value.Type() != attribute.STRING || a.Value.AsString() == "" {
				continue
			}
			a = attribute.String("error.type", "http_error")
			output = append(output, a)
			continue
		case "http.request.method":
			if a.Value.Type() == attribute.STRING {
				switch a.Value.AsString() {
				case "GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH", "_OTHER":
					output = append(output, a)
				}
			}
		case "http.response.status_code":
			if a.Value.Type() == attribute.INT64 && a.Value.AsInt64() >= 100 && a.Value.AsInt64() <= 599 {
				output = append(output, a)
			}
		case "http.request.body.size", "http.response.body.size":
			if a.Value.Type() == attribute.INT64 && a.Value.AsInt64() >= 0 {
				output = append(output, a)
			}
		case "url.scheme":
			if a.Value.Type() == attribute.STRING && (a.Value.AsString() == "http" || a.Value.AsString() == "https") {
				output = append(output, a)
			}
		case "network.protocol.name":
			if a.Value.Type() == attribute.STRING && a.Value.AsString() == "http" {
				output = append(output, a)
			}
		case "network.protocol.version":
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
		if a.Key != "http.request.body.size" && a.Key != "http.response.body.size" {
			result = append(result, a)
		}
	}
	return result
}

type privateTracerProvider struct {
	trace.TracerProvider
	route      string
	scope      string
	start      []trace.SpanStartOption
	attributes []attribute.KeyValue
}

func (p privateTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return privateTracer{Tracer: p.TracerProvider.Tracer(p.scope, opts...), provider: p}
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
		clean = append(clean, trace.WithLinks(trace.Link{SpanContext: link.SpanContext}))
	}
	clean = append(clean, t.provider.start...)
	clean = append(clean, trace.WithAttributes(t.provider.attributes...))
	ctx, span := t.Tracer.Start(ctx, name, clean...)
	wrapped := privateSpan{Span: span, provider: t.provider}
	return trace.ContextWithSpan(ctx, wrapped), wrapped
}

type privateSpan struct {
	trace.Span
	provider trace.TracerProvider
}

func (s privateSpan) TracerProvider() trace.TracerProvider { return s.provider }
func (s privateSpan) SetAttributes(attributes ...attribute.KeyValue) {
	s.Span.SetAttributes(safeAttributes(attributes)...)
}
func (s privateSpan) SetStatus(code codes.Code, _ string)     { s.Span.SetStatus(code, "") }
func (s privateSpan) RecordError(error, ...trace.EventOption) {}
func (s privateSpan) AddEvent(string, ...trace.EventOption)   {}
func (s privateSpan) AddLink(link trace.Link) {
	s.Span.AddLink(trace.Link{SpanContext: link.SpanContext})
}

type privateMeterProvider struct {
	metric.MeterProvider
	scope      string
	attributes []attribute.KeyValue
}

func (p privateMeterProvider) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return privateMeter{Meter: p.MeterProvider.Meter(p.scope, opts...), attributes: p.attributes}
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
	clean := append(safeMetricAttributes(set.ToSlice()), decodeMetricAttributes(set.ToSlice())...)
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

func (m privateMeter) Int64Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	h, err := m.Meter.Int64Counter(name, opts...)
	if err != nil {
		return h, err
	}
	return privateCounter{h, m.attributes}, nil
}
func (m privateMeter) Int64UpDownCounter(name string, opts ...metric.Int64UpDownCounterOption) (metric.Int64UpDownCounter, error) {
	h, err := m.Meter.Int64UpDownCounter(name, opts...)
	if err != nil {
		return h, err
	}
	return privateUpDownCounter{h, m.attributes}, nil
}
func addOptions(opts []metric.AddOption, attrs []attribute.KeyValue) []metric.AddOption {
	c := metric.NewAddConfig(opts)
	set := c.Attributes()
	clean := append(safeMetricAttributes(set.ToSlice()), decodeMetricAttributes(set.ToSlice())...)
	clean = append(clean, attrs...)
	return []metric.AddOption{metric.WithAttributes(clean...)}
}

type privateCounter struct {
	metric.Int64Counter
	attributes []attribute.KeyValue
}

func (c privateCounter) Add(ctx context.Context, value int64, opts ...metric.AddOption) {
	c.Int64Counter.Add(ctx, value, addOptions(opts, c.attributes)...)
}

type privateUpDownCounter struct {
	metric.Int64UpDownCounter
	attributes []attribute.KeyValue
}

func (c privateUpDownCounter) Add(ctx context.Context, value int64, opts ...metric.AddOption) {
	c.Int64UpDownCounter.Add(ctx, value, addOptions(opts, c.attributes)...)
}
