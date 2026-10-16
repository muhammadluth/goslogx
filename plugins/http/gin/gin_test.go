package gin

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPContract(t *testing.T) {
	testkit.Contract(t, ScopeName, "/users/:id", func(p goslogx.TelemetryProvider, next http.Handler, filtered, disabled bool) http.Handler {
		options := []Option{WithMetricAttributes(attribute.String("app.channel", "mobile")), WithTracing(!disabled), WithMetrics(!disabled), WithRequestFilter(func(*http.Request) bool { return !filtered })}
		gin.SetMode(gin.ReleaseMode)
		r := gin.New()
		r.Use(Middleware(p, options...))
		r.GET("/users/:id", func(c *gin.Context) { next.ServeHTTP(c.Writer, c.Request) })
		return r
	})
}

func TestErrorsAndRecoveredPanic(t *testing.T) {
	for _, panicHandler := range []bool{false, true} {
		p := fixture.New(t)
		r := gin.New()
		r.Use(Middleware(p))
		r.Use(gin.CustomRecovery(func(c *gin.Context, _ any) { c.AbortWithStatus(503) }))
		r.GET("/error/:id", func(c *gin.Context) {
			if panicHandler {
				panic("SECRET")
			}
			c.Error(errors.New("SECRET"))
			c.Status(503)
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/error/SECRET", nil))
		if w.Code != 503 {
			t.Fatal(w.Code)
		}
		spans := p.Recorder.Ended()
		if len(spans) != 1 || spans[0].Status().Code != codes.Error {
			t.Fatal("failure span")
		}
		p.AssertPrivate(t, "SECRET")
	}
}
