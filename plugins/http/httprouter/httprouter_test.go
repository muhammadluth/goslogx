package httprouter

import (
	"github.com/julienschmidt/httprouter"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/httptelemetry/testkit"
	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPContract(t *testing.T) {
	testkit.Contract(t, ScopeName, "/users/:id", func(p goslogx.TelemetryProvider, next http.Handler, filtered, disabled bool) http.Handler {
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

func TestHandlePublicEndpointCreatesNewServerRoot(t *testing.T) {
	p := fixture.New(t)
	r := httprouter.New()
	r.GET("/health", Handle(p, "/health", func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) { w.WriteHeader(http.StatusNoContent) }, WithPublicEndpoint()))
	req := httptest.NewRequest(http.MethodGet, "http://service.test/health", nil)
	req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	r.ServeHTTP(httptest.NewRecorder(), req)
	spans := p.Recorder.Ended()
	if len(spans) != 1 || spans[0].SpanKind() != trace.SpanKindServer || spans[0].Parent().IsValid() {
		t.Fatalf("public endpoint must not parent its span from the request: %#v", spans)
	}
}
