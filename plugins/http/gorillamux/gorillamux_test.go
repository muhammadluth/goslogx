package gorillamux

import (
	"github.com/gorilla/mux"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"net/http"
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
