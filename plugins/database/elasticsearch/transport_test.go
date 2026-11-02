package elasticsearch

import (
	"context"
	"fmt"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNativeTransportOperationsAndSignals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("app-header") != "original" {
			t.Error("application headers changed")
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			transport := NewTransport(p, http.DefaultTransport, nil, WithTracing(signals[0]), WithMetrics(signals[1]), WithTraceAttributes(attribute.String("app.component", "search")), WithMetricAttributes(attribute.String("app.region", "test")), WithPropagator(propagation.TraceContext{}))
			for _, request := range [][3]string{{"POST", "/_search", "search"}, {"POST", "/_bulk", "bulk"}, {"POST", "/_update/SECRET", "update"}, {"DELETE", "/index/SECRET", "delete"}, {"PUT", "/_doc/SECRET", "index"}, {"GET", "/", "request"}} {
				req, err := http.NewRequestWithContext(context.Background(), request[0], server.URL+request[1]+"?token=SECRET", nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("app-header", "original")
				res, err := transport.RoundTrip(req)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, res.Body)
				_ = res.Body.Close()
				if res.StatusCode != 503 || req.Header.Get("Traceparent") != "" {
					t.Fatal("native response/caller header behavior changed")
				}
				if signals[0] {
					spans := p.Recorder.Ended()
					last := spans[len(spans)-1]
					if last.Name() != "elasticsearch "+request[2] || last.Status().Code != codes.Error {
						t.Fatal("operation/status", last.Name(), last.Status())
					}
				}
			}
			if (len(p.Recorder.Ended()) == 6) != signals[0] || (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
				t.Fatal("signal modes")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
}
