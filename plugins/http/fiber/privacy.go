package fiber

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	traceAttributePrefix  = "goslogx.trace."
	metricAttributePrefix = "goslogx.metric."
)

type privacyPolicy struct {
	headers  map[attribute.Key]struct{}
	clientIP bool
	port     bool
}

func safeConfiguredAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	result := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		key := strings.ToLower(string(a.Key))
		if key == "" || strings.Contains(key, "password") || strings.Contains(key, "credential") ||
			strings.Contains(key, "authorization") || strings.Contains(key, "token") ||
			strings.Contains(key, "secret") || strings.Contains(key, "cookie") ||
			strings.HasPrefix(key, "http.") || strings.HasPrefix(key, "url.") ||
			strings.HasPrefix(key, "server.") || strings.HasPrefix(key, "client.") || strings.HasPrefix(key, "network.") {
			continue
		}
		if a.Value.Type() == attribute.STRING && len(a.Value.AsString()) > 256 {
			continue
		}
		result = append(result, a)
	}
	return result
}

func encodeTraceAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	return encodeAttributes(traceAttributePrefix, safeConfiguredAttributes(input))
}

func encodeMetricAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	approved := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		switch a.Key {
		case "app.channel", "app.region", "app.tier", "service.version", "deployment.environment.name":
			if a.Value.Type() == attribute.STRING && len(a.Value.AsString()) <= 128 {
				approved = append(approved, a)
			}
		}
	}
	return encodeAttributes(metricAttributePrefix, approved)
}

func encodeAttributes(prefix string, input []attribute.KeyValue) []attribute.KeyValue {
	result := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		result = append(result, attribute.KeyValue{Key: attribute.Key(prefix + string(a.Key)), Value: a.Value})
	}
	return result
}

func decodeAttributes(prefix string, input []attribute.KeyValue) []attribute.KeyValue {
	result := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		key := string(a.Key)
		if strings.HasPrefix(key, prefix) {
			result = append(result, attribute.KeyValue{Key: attribute.Key(strings.TrimPrefix(key, prefix)), Value: a.Value})
		}
	}
	return result
}

func method(s string) string {
	switch s {
	case "GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH":
		return s
	}
	return "_OTHER"
}

func safeAttributes(input []attribute.KeyValue, policy privacyPolicy) []attribute.KeyValue {
	output := make([]attribute.KeyValue, 0, len(input))
	output = append(output, safeConfiguredAttributes(decodeAttributes(traceAttributePrefix, input))...)
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
		case "server.port":
			if !policy.port || a.Value.Type() != attribute.INT64 || a.Value.AsInt64() <= 0 || a.Value.AsInt64() > 65535 {
				continue
			}
		case "client.address", "network.peer.address":
			if !policy.clientIP || a.Value.Type() != attribute.STRING || a.Value.AsString() == "" {
				continue
			}
		case "network.peer.port":
			if !policy.clientIP || a.Value.Type() != attribute.INT64 || a.Value.AsInt64() <= 0 || a.Value.AsInt64() > 65535 {
				continue
			}
		default:
			if _, ok := policy.headers[a.Key]; !ok || a.Value.Type() != attribute.STRINGSLICE {
				continue
			}
		}
		output = append(output, a)
	}
	return output
}

// Body-size measurements belong in histograms, not metric dimensions.
func safeMetricAttributes(input []attribute.KeyValue) []attribute.KeyValue {
	attributes := safeAttributes(input, privacyPolicy{})
	attributes = append(attributes, decodeAttributes(metricAttributePrefix, input)...)
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
	attributes []attribute.KeyValue
	start      []trace.SpanStartOption
	policy     privacyPolicy
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
	attrs := safeAttributes(c.Attributes(), t.provider.policy)
	if t.provider.serverName != "" {
		attrs = append(attrs, attribute.String("server.address", t.provider.serverName))
	}
	clean := []trace.SpanStartOption{trace.WithAttributes(attrs...), trace.WithSpanKind(c.SpanKind()), trace.WithTimestamp(c.Timestamp())}
	if c.NewRoot() {
		clean = append(clean, trace.WithNewRoot())
	}
	for _, link := range c.Links() {
		clean = append(clean, trace.WithLinks(trace.Link{SpanContext: link.SpanContext}))
	}
	clean = append(clean, t.provider.start...)
	clean = append(clean, trace.WithAttributes(t.provider.attributes...))
	ctx, span := t.Tracer.Start(ctx, method(name), clean...)
	wrapped := privateSpan{Span: span, provider: t.provider}
	return trace.ContextWithSpan(ctx, wrapped), wrapped
}

type privateSpan struct {
	trace.Span
	provider privateTracerProvider
}

func (s privateSpan) TracerProvider() trace.TracerProvider { return s.provider }
func (s privateSpan) SetAttributes(attrs ...attribute.KeyValue) {
	s.Span.SetAttributes(safeAttributes(attrs, s.provider.policy)...)
}
func (s privateSpan) SetStatus(code codes.Code, _ string)     { s.Span.SetStatus(code, "") }
func (s privateSpan) RecordError(error, ...trace.EventOption) {}
func (s privateSpan) AddEvent(string, ...trace.EventOption)   {}
func (s privateSpan) AddLink(link trace.Link) {
	s.Span.AddLink(trace.Link{SpanContext: link.SpanContext})
}
func (s privateSpan) End(...trace.SpanEndOption) { s.Span.End() }

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
