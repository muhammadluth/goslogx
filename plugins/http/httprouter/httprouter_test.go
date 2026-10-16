package httprouter

import (
	"github.com/julienschmidt/httprouter"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"net/http"
	"testing"
)

func TestHTTPContract(t *testing.T) {
	testkit.Contract(t, ScopeName, "/users/:id", func(p goslogx.TelemetryProvider, next http.Handler, filtered, disabled bool) http.Handler {
		options := []Option{WithMetricAttributes(attribute.String("app.channel", "mobile")), WithTracing(!disabled), WithMetrics(!disabled), WithRequestFilter(func(*http.Request) bool { return !filtered })}
		r := httprouter.New()
		r.GET("/users/:id", Handle(p, "/users/:id", func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
			if params.ByName("id") == "" {
				t.Error("lost params")
			}
			next.ServeHTTP(w, r)
		}, options...))
		return r
	})
}
