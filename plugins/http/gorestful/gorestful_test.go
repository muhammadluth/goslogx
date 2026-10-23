package gorestful

import (
	"github.com/emicklei/go-restful/v3"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPContract(t *testing.T) {
	testkit.Contract(t, ScopeName, "/users/{id}", func(p goslogx.TelemetryProvider, next http.Handler, filtered, disabled bool) http.Handler {
		options := []Option{WithMetricAttributes(attribute.String("app.channel", "mobile")), WithTracing(!disabled), WithMetrics(!disabled), WithRequestFilter(func(*http.Request) bool { return !filtered })}
		r := restful.NewContainer()
		f, err := NewFilter(p, options...)
		if err != nil {
			t.Fatal(err)
		}
		r.Filter(f)
		ws := new(restful.WebService)
		ws.Route(ws.GET("/users/{id}").To(func(req *restful.Request, resp *restful.Response) { next.ServeHTTP(resp, req.Request) }))
		r.Add(ws)
		return r
	})
}

func TestPublicEndpointFnParity(t *testing.T) {
	p := fixture.New(t)
	calls := 0
	filter, err := NewFilter(p, WithPublicEndpointFn(func(*http.Request) bool { calls++; return false }))
	if err != nil {
		t.Fatal(err)
	}
	container := restful.NewContainer()
	container.Filter(filter)
	service := new(restful.WebService)
	service.Route(service.GET("/health").To(func(_ *restful.Request, response *restful.Response) { response.WriteHeader(204) }))
	container.Add(service)
	container.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/health", nil))
	if calls != 1 || len(p.Recorder.Ended()) != 1 {
		t.Fatalf("public endpoint callback parity: calls=%d spans=%d", calls, len(p.Recorder.Ended()))
	}
}
