package gin

import (
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPContract(t *testing.T) {
	testkit.Contract(t, ScopeName, "/users/:id", func(p goslogx.TelemetryProvider, next http.Handler, filtered, disabled bool) http.Handler {
		options := []Option{
			WithServiceName("users-api"),
			WithTraceAttributes(attribute.String("app.component", "handler")),
			WithSpanStartOptions(trace.WithAttributes(attribute.String("app.start", "safe"))),
			WithSpanNameFormatter(func(route string, _ *http.Request) string { return "HTTP " + route }),
			WithMetricAttributes(attribute.String("app.channel", "mobile")),
			WithMetricAttributesFn(func(*http.Request) []attribute.KeyValue {
				return []attribute.KeyValue{attribute.String("app.tier", "edge")}
			}),
			WithGinFilter(func(*gin.Context) bool { return true }),
			WithTracing(!disabled), WithMetrics(!disabled),
			WithRequestFilter(func(*http.Request) bool { return !filtered }),
		}
		gin.SetMode(gin.ReleaseMode)
		r := gin.New()
		r.Use(Middleware(p, options...))
		r.GET("/users/:id", func(c *gin.Context) { next.ServeHTTP(c.Writer, c.Request) })
		return r
	})
}

func TestGinSpecificCapabilityOptions(t *testing.T) {
	t.Run("gin filter", func(t *testing.T) {
		p := fixture.New(t)
		called := false
		r := gin.New()
		r.Use(Middleware(p, WithGinFilter(func(*gin.Context) bool { return false })))
		r.GET("/filtered", func(c *gin.Context) { called = true; c.Status(204) })
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/filtered", nil))
		if !called || len(p.Recorder.Ended()) != 0 || len(p.Collect(t).ScopeMetrics) != 0 {
			t.Fatal("gin filter parity")
		}
	})

	t.Run("dynamic metric attributes", func(t *testing.T) {
		p := fixture.New(t)
		r := gin.New()
		r.Use(Middleware(p,
			WithMetricAttributeFn(func(*http.Request) []attribute.KeyValue {
				return []attribute.KeyValue{attribute.String("app.region", "apac"), attribute.String("app.user_id", "SECRET")}
			}),
			WithGinMetricAttributeFn(func(*gin.Context) []attribute.KeyValue {
				return []attribute.KeyValue{attribute.String("app.channel", "mobile")}
			}),
		))
		r.GET("/metrics", func(c *gin.Context) { c.Status(204) })
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/metrics", nil))
		data := fmt.Sprint(p.Collect(t))
		if !strings.Contains(data, "apac") || !strings.Contains(data, "mobile") || strings.Contains(data, "SECRET") {
			t.Fatalf("dynamic attributes not preserved safely: %s", data)
		}
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
