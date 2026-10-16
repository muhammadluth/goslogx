package httptelemetry

import (
	"context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"strconv"
	"time"
)

// ServerMetrics fills only the metrics absent from upstream instrumentation.
type ServerMetrics struct {
	duration metric.Float64Histogram
	active   metric.Int64UpDownCounter
	attrs    []attribute.KeyValue
}

func NewServerMetrics(p Provider, c Config) (*ServerMetrics, error) {
	m := p.MP.Meter("HTTP")
	duration, err := m.Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.005, .01, .025, .05, .075, .1, .25, .5, .75, 1, 2.5, 5, 7.5, 10))
	if err != nil {
		return nil, err
	}
	active, err := m.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"))
	if err != nil {
		return nil, err
	}
	return &ServerMetrics{duration, active, c.MetricAttributes}, nil
}
func (m *ServerMetrics) Begin(ctx context.Context, method string) func(string, int, bool) {
	start := time.Now()
	attrs := append([]attribute.KeyValue{attribute.String("http.request.method", Method(method))}, m.attrs...)
	m.active.Add(ctx, 1, metric.WithAttributes(attrs...))
	return func(route string, status int, failed bool) {
		m.active.Add(ctx, -1, metric.WithAttributes(attrs...))
		if route != "" && len(route) <= 256 {
			attrs = append(attrs, attribute.String("http.route", route))
		}
		if status >= 100 && status <= 599 {
			attrs = append(attrs, attribute.Int("http.response.status_code", status))
		}
		if failed || status >= 500 {
			value := "http_error"
			if status >= 500 && status <= 599 {
				value = strconv.Itoa(status)
			}
			attrs = append(attrs, attribute.String("error.type", value))
		}
		m.duration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attrs...))
	}
}
