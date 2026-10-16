package gorestful

import (
	"github.com/emicklei/go-restful/v3"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"net/http"
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
