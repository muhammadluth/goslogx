package elasticsearch

import (
	"context"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDirectTransportParity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("traceparent") == "" {
			t.Error("missing propagation")
		}
		w.WriteHeader(503)
		_, _ = w.Write([]byte("response"))
	}))
	defer server.Close()
	var spans [2]sdktrace.ReadOnlySpan
	for index, adapter := range []bool{false, true} {
		p := testkit.New(t)
		var transport http.RoundTripper
		if adapter {
			transport = NewTransport(p, server.Client().Transport)
		} else {
			transport = otelhttp.NewTransport(server.Client().Transport, otelhttp.WithTracerProvider(p.TP), otelhttp.WithMeterProvider(p.MP), otelhttp.WithPropagators(p.Propagator), otelhttp.WithSpanNameFormatter(func(string, *http.Request) string { return "elasticsearch search" }))
		}
		sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
		req, _ := http.NewRequestWithContext(trace.ContextWithSpanContext(context.Background(), sc), "POST", server.URL+"/orders/_search?token=SECRET", nil)
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Recorder.Ended()) != 0 {
			t.Fatal("span ended before response consumption")
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err = resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 503 || string(body) != "response" {
			t.Fatal("response changed")
		}
		ended := p.Recorder.Ended()
		if len(ended) != 1 {
			t.Fatal("duplicate or missing transport span", ended)
		}
		spans[index] = ended[0]
		if adapter {
			p.AssertPrivate(t, "SECRET")
		}
	}
	want, got := spans[0], spans[1]
	if got.Name() != want.Name() || got.SpanKind() != want.SpanKind() || got.Status().Code != want.Status().Code || !got.Parent().Equal(want.Parent()) {
		t.Fatal("transport semantics changed")
	}
}
