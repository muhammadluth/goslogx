package std

import (
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"net/http"
	"testing"
)

func TestHTTPContract(t *testing.T) {
	testkit.Contract(t, ScopeName, "GET /users/{id}", func(p goslogx.TelemetryProvider, next http.Handler, filtered, disabled bool) http.Handler {
		mux := http.NewServeMux()
		mux.Handle("GET /users/{id}", next)
		return NewHandler(p, mux, "api", WithTracing(!disabled), WithMetrics(!disabled), WithMetricAttributes(attribute.String("app.channel", "mobile")), WithRequestFilter(func(*http.Request) bool { return !filtered }))
	})
}
