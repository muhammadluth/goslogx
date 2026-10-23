package chi

import (
	"github.com/go-chi/chi/v5"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"net/http"
	"testing"
)

func TestHTTPContract(t *testing.T) {
	testkit.Contract(t, ScopeName, "/users/{id}", func(p goslogx.TelemetryProvider, next http.Handler, filtered, disabled bool) http.Handler {
		options := []Option{
			WithServerName("users-api"), WithMetricAttributes(attribute.String("app.channel", "mobile")),
			WithMetricAttributesFn(func(*http.Request) []attribute.KeyValue {
				return []attribute.KeyValue{attribute.String("app.tier", "edge")}
			}),
			WithPublicEndpointFn(func(*http.Request) bool { return false }),
			WithSpanNameFormatter(func(route string, _ *http.Request) string { return "HTTP " + route }),
			WithMessageEvents(ReadEvent, WriteEvent), WithTracing(!disabled), WithMetrics(!disabled),
			WithRequestFilter(func(*http.Request) bool { return !filtered }),
		}
		r := chi.NewRouter()
		r.Use(Middleware(p, options...))
		r.Get("/users/{id}", next.ServeHTTP)
		return r
	})
}
