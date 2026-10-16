package fasthttp

import (
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	native "github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestHTTPContract(t *testing.T) {
	testkit.Contract(t, ScopeName, "/users/:id", func(p goslogx.TelemetryProvider, next http.Handler, filtered, disabled bool) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h, err := NewHandler(p, func(c *native.RequestCtx) {
				record := httptest.NewRecorder()
				next.ServeHTTP(record, r.WithContext(Context(c)))
				c.SetStatusCode(record.Code)
			}, WithRoute("/users/:id"), WithTracing(!disabled), WithMetrics(!disabled), WithMetricAttributes(attribute.String("app.channel", "mobile")), WithFilter(func(*native.RequestCtx) bool { return !filtered }))
			if err != nil {
				t.Fatal(err)
			}
			var req native.Request
			req.SetRequestURI(r.URL.String())
			req.Header.SetMethod(r.Method)
			for key, values := range r.Header {
				for _, v := range values {
					req.Header.Add(key, v)
				}
			}
			var ctx native.RequestCtx
			ctx.Init(&req, nil, nil)
			h(&ctx)
			w.WriteHeader(ctx.Response.StatusCode())
		})
	})
}

func TestPanicAndContextRestoration(t *testing.T) {
	p := fixture.New(t)
	h, err := NewHandler(p, func(c *native.RequestCtx) { panic("SECRET") })
	if err != nil {
		t.Fatal(err)
	}
	var req native.Request
	req.SetRequestURI("http://localhost/SECRET")
	var ctx native.RequestCtx
	ctx.Init(&req, nil, nil)
	func() {
		defer func() {
			if recover() != "SECRET" {
				t.Error("panic changed")
			}
		}()
		h(&ctx)
	}()
	if Context(&ctx).Err() != nil || trace.SpanContextFromContext(Context(&ctx)).IsValid() {
		t.Fatal("invalid restored context")
	}
	spans := p.Recorder.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatal("panic not observed")
	}
	p.AssertPrivate(t, "SECRET")
}

func TestConcurrentNativeRequests(t *testing.T) {
	p := fixture.New(t)
	h, err := NewHandler(p, func(c *native.RequestCtx) {
		if !trace.SpanContextFromContext(Context(c)).IsValid() {
			t.Error("missing active context")
		}
		c.SetStatusCode(204)
	}, WithRoute("/orders/:id"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var req native.Request
			req.SetRequestURI("http://localhost/orders/SECRET")
			req.Header.SetMethod("GET")
			var ctx native.RequestCtx
			ctx.Init(&req, nil, nil)
			h(&ctx)
			if trace.SpanContextFromContext(Context(&ctx)).IsValid() {
				t.Error("retained request span")
			}
		}()
	}
	wg.Wait()
	if len(p.Recorder.Ended()) != 16 {
		t.Fatal("lost request spans")
	}
	p.AssertPrivate(t, "SECRET")
}
