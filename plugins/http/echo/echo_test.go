package echo

import (
	"errors"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPContract(t *testing.T) {
	testkit.Contract(t, ScopeName, "/users/:id", func(p goslogx.TelemetryProvider, next http.Handler, filtered, disabled bool) http.Handler {
		options := []Option{
			WithServiceName("users-api"), WithTraceAttributes(attribute.String("app.component", "handler")),
			WithMetricAttributes(attribute.String("app.channel", "mobile")),
			WithMetricAttributesFn(func(*http.Request) []attribute.KeyValue {
				return []attribute.KeyValue{attribute.String("app.tier", "edge")}
			}),
			WithEchoMetricAttributesFn(func(echo.Context) []attribute.KeyValue {
				return []attribute.KeyValue{attribute.String("app.region", "apac")}
			}),
			WithSkipper(func(echo.Context) bool { return false }), WithOnError(func(echo.Context, error) {}),
			WithTracing(!disabled), WithMetrics(!disabled), WithRequestFilter(func(*http.Request) bool { return !filtered }),
		}
		r := echo.New()
		r.Use(Middleware(p, options...))
		r.GET("/users/:id", func(c echo.Context) error { next.ServeHTTP(c.Response(), c.Request()); return nil })
		return r
	})
}

func TestEchoSpecificCapabilityOptions(t *testing.T) {
	t.Run("skipper", func(t *testing.T) {
		p := fixture.New(t)
		called := false
		e := echo.New()
		e.Use(Middleware(p, WithSkipper(func(echo.Context) bool { return true })))
		e.GET("/skip", func(c echo.Context) error { called = true; return c.NoContent(204) })
		e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/skip", nil))
		if !called || len(p.Recorder.Ended()) != 0 || len(p.Collect(t).ScopeMetrics) != 0 {
			t.Fatal("skipper parity")
		}
	})

	t.Run("dynamic metric attributes", func(t *testing.T) {
		p := fixture.New(t)
		e := echo.New()
		e.Use(Middleware(p,
			WithMetricAttributeFn(func(*http.Request) []attribute.KeyValue {
				return []attribute.KeyValue{attribute.String("app.region", "apac"), attribute.String("app.user_id", "SECRET")}
			}),
			WithEchoMetricAttributeFn(func(echo.Context) []attribute.KeyValue {
				return []attribute.KeyValue{attribute.String("app.channel", "mobile")}
			}),
		))
		e.GET("/metrics", func(c echo.Context) error { return c.NoContent(204) })
		e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/metrics", nil))
		data := fmt.Sprint(p.Collect(t))
		if !strings.Contains(data, "apac") || !strings.Contains(data, "mobile") || strings.Contains(data, "SECRET") {
			t.Fatalf("dynamic attributes not preserved safely: %s", data)
		}
	})
}

func TestApplicationErrorHandler(t *testing.T) {
	for _, filtered := range []bool{false, true} {
		t.Run(fmt.Sprint(filtered), func(t *testing.T) {
			p := fixture.New(t)
			e := echo.New()
			calls := 0
			sentinel := errors.New("SECRET")
			e.HTTPErrorHandler = func(err error, c echo.Context) {
				calls++
				if err != sentinel {
					t.Error("error identity")
				}
				c.String(503, "application response")
			}
			e.Use(Middleware(p, WithFilter(func(*http.Request) bool { return !filtered })))
			e.GET("/error/:id", func(echo.Context) error { return sentinel })
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("GET", "/error/SECRET", nil))
			if calls != 1 || w.Code != 503 || w.Body.String() != "application response" {
				t.Fatal("custom handler changed", calls, w.Code, w.Body.String())
			}
			spans := p.Recorder.Ended()
			if filtered {
				if len(spans) != 0 {
					t.Fatal("filter")
				}
			} else if len(spans) != 1 || spans[0].Status().Code != codes.Error {
				t.Fatal("error status")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
}
