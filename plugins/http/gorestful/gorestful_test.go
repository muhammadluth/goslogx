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

func TestPublicEndpointDoesNotUseRemoteParent(t *testing.T) {
	p := fixture.New(t)
	filter, err := NewFilter(p, WithPublicEndpoint())
	if err != nil {
		t.Fatal(err)
	}
	container := restful.NewContainer()
	container.Filter(filter)
	service := new(restful.WebService)
	service.Route(service.GET("/public").To(func(_ *restful.Request, response *restful.Response) { response.WriteHeader(204) }))
	container.Add(service)
	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	container.ServeHTTP(httptest.NewRecorder(), req)
	spans := p.Recorder.Ended()
	if len(spans) != 1 || spans[0].Parent().IsValid() {
		t.Fatalf("public endpoint used request parent: %#v", spans)
	}
}
