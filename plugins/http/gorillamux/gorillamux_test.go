package gorillamux

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPContract(t *testing.T) {
	testkit.Contract(t, ScopeName, "/users/{id}", func(p goslogx.TelemetryProvider, next http.Handler, filtered, disabled bool) http.Handler {
		options := []Option{WithMetricAttributes(attribute.String("app.channel", "mobile")), WithTracing(!disabled), WithMetrics(!disabled), WithRequestFilter(func(*http.Request) bool { return !filtered })}
		r := mux.NewRouter()
		r.Use(Middleware(p, options...))
		r.Handle("/users/{id}", next).Methods("GET")
		return r
	})
}

func TestUpstreamCapabilityOptions(t *testing.T) {
	p := fixture.New(t)
	publicCalls := 0
	r := mux.NewRouter()
	r.Use(Middleware(p,
		WithPublicEndpointFn(func(*http.Request) bool { publicCalls++; return false }),
		WithMetricAttributesFn(func(*http.Request) []attribute.KeyValue {
			return []attribute.KeyValue{attribute.String("app.channel", "mobile"), attribute.String("app.user_id", "SECRET")}
		}),
	))
	r.HandleFunc("/metrics/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }).Methods("GET")
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/metrics/1", nil))
	data := fmt.Sprint(p.Collect(t))
	if publicCalls != 1 || !strings.Contains(data, "mobile") || strings.Contains(data, "SECRET") {
		t.Fatalf("upstream options not preserved safely: calls=%d metrics=%s", publicCalls, data)
	}
}
