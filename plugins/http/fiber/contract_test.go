package fiber

import (
	native "github.com/gofiber/fiber/v3"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPContract(t *testing.T) {
	testkit.Contract(t, ScopeName, "/users/:id", func(p goslogx.TelemetryProvider, next http.Handler, filtered, disabled bool) http.Handler {
		app := native.New()
		app.Use(Middleware(p, WithTracing(!disabled), WithMetrics(!disabled), WithMetricAttributes(attribute.String("app.channel", "mobile")), WithRequestFilter(func(native.Ctx) bool { return !filtered })))
		app.Get("/users/:id", func(c native.Ctx) error {
			r, _ := http.NewRequestWithContext(c.Context(), "GET", "http://localhost"+c.OriginalURL(), nil)
			w := httptest.NewRecorder()
			next.ServeHTTP(w, r)
			c.Status(w.Code)
			return c.Send(w.Body.Bytes())
		})
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res, err := app.Test(r)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			w.WriteHeader(res.StatusCode)
			io.Copy(w, res.Body)
		})
	})
}
