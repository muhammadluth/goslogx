package chi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	native "github.com/go-chi/chi/v5"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func TestDelegationKeepsCustomTelemetryAndMessageEvents(t *testing.T) {
	p := testkit.New(t)
	router := native.NewRouter()
	router.Use(Middleware(p, WithTraceAttributes(attribute.String("app.component", "web")), WithSpanStartOptions(trace.WithAttributes(attribute.String("app.operation", "lookup"))), WithMetricAttributesFn(func(*http.Request) []attribute.KeyValue {
		return []attribute.KeyValue{attribute.String("app.tier", "edge"), attribute.String("url.query", "SECRET")}
	}), WithMessageEvents(ReadEvent, WriteEvent)))
	router.Post("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("ok"))
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/users/123?password=SECRET", strings.NewReader("SECRET")))
	spans := p.Recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("native read/write events lost: %v", spans)
	}
	var readBytes, writeBytes int64
	for _, event := range spans[0].Events() {
		for _, a := range event.Attributes {
			if event.Name == "read" && a.Key == "http.read_bytes" {
				readBytes += a.Value.AsInt64()
			}
			if event.Name == "write" && a.Key == "http.wrote_bytes" {
				writeBytes += a.Value.AsInt64()
			}
		}
	}
	if readBytes != 6 || writeBytes != 2 {
		t.Fatalf("native message sizes lost: %v", spans[0].Events())
	}
	attrs := fmt.Sprint(spans[0].Attributes())
	for _, want := range []string{"app.component", "web", "app.operation", "lookup"} {
		if !strings.Contains(attrs, want) {
			t.Fatal("configured span attribute dropped", attrs)
		}
	}
	if metrics := fmt.Sprint(p.Collect(t)); !strings.Contains(metrics, "app.tier") || !strings.Contains(metrics, "edge") {
		t.Fatal("configured metric callback was filtered twice", metrics)
	}
	p.AssertPrivate(t, "SECRET")
}
