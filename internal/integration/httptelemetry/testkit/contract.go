// Package testkit verifies the shared HTTP adapter contract without external services.
package testkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/muhammadluth/goslogx"
	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Builder registers GET /users/{id} (or the framework equivalent). A filter flag
// rejects the request. Disabled turns off both signals without dropping the handler.
type Builder func(goslogx.TelemetryProvider, http.Handler, bool, bool) http.Handler

func Contract(t *testing.T, scope, route string, build Builder) {
	p := fixture.New(t)
	var logs bytes.Buffer
	logger := goslogx.New(goslogx.WithOutput(&logs), goslogx.WithTracerProvider(p.TP), goslogx.WithMeterProvider(p.MP))
	for _, status := range []int{200, 404, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			p.Recorder.Reset()
			logs.Reset()
			var handlerSC trace.SpanContext
			h := build(withPropagator{logger, propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerSC = trace.SpanContextFromContext(r.Context())
				if baggage.FromContext(r.Context()).Member("region").Value() != "contract" {
					t.Error("custom provider propagator not used")
				}
				logger.InfoContext(r.Context(), "", "handler", goslogx.MESSSAGE_TYPE_REQUEST, "received", nil)
				_, child := p.TP.Tracer("downstream").Start(r.Context(), "child")
				child.End()
				w.WriteHeader(status)
			}), false, false)
			req := httptest.NewRequest("GET", "http://localhost/users/SECRET?token=SECRET", nil)
			req.Header.Set("baggage", "region=contract")
			req.Header.Set("Authorization", "SECRET")
			req.Header.Set("Cookie", "session=SECRET")
			req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
			response := httptest.NewRecorder()
			h.ServeHTTP(response, req)
			if response.Code != status {
				t.Fatal("response changed", response.Code)
			}
			spans := p.Recorder.Ended()
			if len(spans) != 2 {
				t.Fatalf("want one server and one child, got %v", spans)
			}
			var server, childID trace.SpanContext
			for _, s := range spans {
				if s.SpanKind() == trace.SpanKindServer {
					server = s.SpanContext()
					if s.InstrumentationScope().Name != scope {
						t.Fatal("scope", s.InstrumentationScope().Name)
					}
					if !s.Parent().IsRemote() || s.Parent().SpanID().String() != "0123456789abcdef" {
						t.Fatal("remote parent")
					}
					found := false
					for _, a := range s.Attributes() {
						if string(a.Key) == "http.route" && a.Value.AsString() == route {
							found = true
						}
					}
					if !found {
						t.Fatal("missing route", s.Attributes())
					}
					if (s.Status().Code == codes.Error) != (status >= 500) {
						t.Fatal("HTTP error semantics", s.Status())
					}
				} else {
					childID = s.Parent()
				}
			}
			if !server.IsValid() || !handlerSC.Equal(server) || childID.SpanID() != server.SpanID() || server.TraceID().String() != "0123456789abcdef0123456789abcdef" {
				t.Fatal("context continuity")
			}
			var entry map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
				t.Fatal(err)
			}
			if entry["obs_trace_id"] != server.TraceID().String() || entry["obs_span_id"] != server.SpanID().String() {
				t.Fatal("log correlation")
			}
			data := p.Collect(t)
			text := fmt.Sprint(data)
			if !strings.Contains(text, "http.server.request.duration") || !strings.Contains(text, route) || !strings.Contains(text, "mobile") {
				t.Fatal("missing route/custom HTTP metrics", text)
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	t.Run("filter", func(t *testing.T) {
		before := counts(p.Collect(t))
		p.Recorder.Reset()
		called := false
		h := build(p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) }), true, false)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/users/SECRET", nil))
		if !called || len(p.Recorder.Ended()) != 0 || counts(p.Collect(t)) != before {
			t.Fatal("filter behavior")
		}
	})
	t.Run("disabled", func(t *testing.T) {
		before := counts(p.Collect(t))
		p.Recorder.Reset()
		called := false
		h := build(p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }), false, true)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/users/SECRET", nil))
		if !called || len(p.Recorder.Ended()) != 0 || counts(p.Collect(t)) != before {
			t.Fatal("disabled telemetry")
		}
	})
	t.Run("missing-provider", func(t *testing.T) {
		h := build(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), false, false)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/users/SECRET", nil))
	})
}

type withPropagator struct {
	goslogx.TelemetryProvider
	prop propagation.TextMapPropagator
}

func (p withPropagator) TextMapPropagator() propagation.TextMapPropagator { return p.prop }
func counts(data metricdata.ResourceMetrics) uint64 {
	var total uint64
	for _, s := range data.ScopeMetrics {
		for _, m := range s.Metrics {
			switch d := m.Data.(type) {
			case metricdata.Histogram[float64]:
				for _, v := range d.DataPoints {
					total += v.Count
				}
			case metricdata.Histogram[int64]:
				for _, v := range d.DataPoints {
					total += v.Count
				}
			}
		}
	}
	return total
}
