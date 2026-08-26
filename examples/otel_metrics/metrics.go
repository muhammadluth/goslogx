// Package otelmetrics contains copy-and-adapt application metric examples.
// It is deliberately separate from goslogx so the logging library does not
// depend on HTTP routers or application business domains.
package otelmetrics

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/muhammadluth/goslogx"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const paymentCreatedEvent = "payment.created"

// RoutePattern returns the matched, low-cardinality route template for a
// request, such as /payments/{id}. It must never return r.URL.Path when the
// path includes user-provided identifiers.
type RoutePattern func(*http.Request) string

// HTTPREDMetrics records application-owned HTTP RED metrics through the
// MeterProvider injected into goslogx. The application owns the provider and
// must flush and shut it down during application shutdown.
type HTTPREDMetrics struct {
	routePattern RoutePattern
	requests     metric.Int64Counter
	errors       metric.Int64Counter
	duration     metric.Float64Histogram
	events       metric.Int64Counter
}

// NewHTTPREDMetrics creates HTTP RED and payment business-event instruments.
// instrumentationScope should identify the application instrumentation, for
// example "payment-api".
func NewHTTPREDMetrics(logger *goslogx.Logger, instrumentationScope string, routePattern RoutePattern) (*HTTPREDMetrics, error) {
	if logger == nil {
		return nil, errors.New("otel metrics: logger is required")
	}
	if instrumentationScope == "" {
		return nil, errors.New("otel metrics: instrumentation scope is required")
	}

	meter := logger.MeterProvider().Meter(instrumentationScope)
	requests, err := meter.Int64Counter("http.server.requests")
	if err != nil {
		return nil, err
	}
	errors, err := meter.Int64Counter("http.server.errors")
	if err != nil {
		return nil, err
	}
	duration, err := meter.Float64Histogram("http.server.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	events, err := meter.Int64Counter("payment.events")
	if err != nil {
		return nil, err
	}

	return &HTTPREDMetrics{
		routePattern: routePattern,
		requests:     requests,
		errors:       errors,
		duration:     duration,
		events:       events,
	}, nil
}

// Middleware records request count, 5xx error count, and request duration.
// It passes r.Context to OTel so a sampled active span can be attached as an
// exemplar by the configured MeterProvider.
func (m *HTTPREDMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		response := &statusResponseWriter{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(response, request)

		attributes := []attribute.KeyValue{
			attribute.String("http.request.method", request.Method),
		}
		if m.routePattern != nil {
			if route := m.routePattern(request); route != "" {
				attributes = append(attributes, attribute.String("http.route", route))
			}
		}

		m.requests.Add(request.Context(), 1, metric.WithAttributes(attributes...))
		if response.status >= http.StatusInternalServerError {
			m.errors.Add(request.Context(), 1, metric.WithAttributes(attributes...))
		}
		m.duration.Record(
			request.Context(),
			time.Since(started).Seconds(),
			metric.WithAttributes(attributes...),
		)
	})
}

// RecordPaymentCreated records a bounded business event. It intentionally does
// not accept a payment ID or arbitrary event name as a metric attribute.
func (m *HTTPREDMetrics) RecordPaymentCreated(ctx context.Context) {
	if m == nil {
		return
	}
	m.events.Add(
		ctx,
		1,
		metric.WithAttributes(attribute.String("event.name", paymentCreatedEvent)),
	)
}

type statusResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.status = status
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

// Unwrap lets http.ResponseController reach optional capabilities implemented
// by the original ResponseWriter.
func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
