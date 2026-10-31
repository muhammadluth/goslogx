package gorillamux

import (
	"fmt"
	native "github.com/gorilla/mux"
	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFallbackRouterOutcomes(t *testing.T) {
	for _, tracing := range []bool{false, true} {
		for _, metrics := range []bool{false, true} {
			for _, filtered := range []bool{false, true} {
				t.Run(fmt.Sprintf("trace=%v/metrics=%v/filter=%v", tracing, metrics, filtered), func(t *testing.T) {
					p := fixture.New(t)
					opts := []Option{WithTracing(tracing), WithMetrics(metrics), WithRequestFilter(func(*http.Request) bool { return !filtered })}
					r := native.NewRouter()
					r.Use(Middleware(p, opts...))
					r.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }).Methods("GET")
					r.NotFoundHandler = Fallback(p, nil, opts...)
					r.MethodNotAllowedHandler = Fallback(p, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(405) }), opts...)
					for i, path := range []string{"/users/SECRET-id", "/missing/SECRET-id", "/users/SECRET-id"} {
						method := "GET"
						want := 204
						if i == 1 {
							want = 404
						}
						if i == 2 {
							method = "POST"
							want = 405
						}
						req := httptest.NewRequest(method, path, nil)
						req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
						w := httptest.NewRecorder()
						r.ServeHTTP(w, req)
						if w.Code != want {
							t.Fatalf("response=%d want=%d", w.Code, want)
						}
					}
					spans := p.Recorder.Ended()
					wantSpans := 0
					if tracing && !filtered {
						wantSpans = 3
					}
					if len(spans) != wantSpans {
						t.Fatalf("span count=%d want=%d", len(spans), wantSpans)
					}
					for _, s := range spans {
						if s.SpanKind() != trace.SpanKindServer || s.Parent().SpanID().String() != "0123456789abcdef" {
							t.Fatalf("span relationship: %v", s)
						}
					}
					data := p.Collect(t)
					histograms := 0
					for _, scope := range data.ScopeMetrics {
						for _, m := range scope.Metrics {
							switch h := m.Data.(type) {
							case metricdata.Histogram[float64]:
								histograms++
								var count uint64
								for _, pt := range h.DataPoints {
									count += pt.Count
								}
								if count != 3 {
									t.Fatalf("%s count=%d", m.Name, count)
								}
							case metricdata.Histogram[int64]:
								histograms++
								var count uint64
								for _, pt := range h.DataPoints {
									count += pt.Count
								}
								if count != 3 {
									t.Fatalf("%s count=%d", m.Name, count)
								}
							}
						}
					}
					if metrics && !filtered && histograms == 0 {
						t.Fatal("missing native metrics")
					}
					if (!metrics || filtered) && histograms != 0 {
						t.Fatal("unexpected metrics")
					}
					p.AssertPrivate(t, "SECRET-id")
				})
			}
		}
	}
}
