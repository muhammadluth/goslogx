package opensearch

import (
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestTransportPrivacyAndProviders(t *testing.T) {
	p := testkit.New(t)
	transport := NewTransport(p, transportFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "SECRET" {
			t.Fatal("request body changed")
		}
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}))
	req := httptest.NewRequest("POST", "http://user:SECRET@localhost/index.SECRET/_search?token=SECRET", strings.NewReader("SECRET"))
	res, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	spans := p.Recorder.Ended()
	if len(spans) != 1 || spans[0].Name() != "opensearch search" || spans[0].InstrumentationScope().Name != ScopeName {
		t.Fatal("scope/operation")
	}
	p.AssertPrivate(t, "SECRET")
	if len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("metrics")
	}
}
