package otelmetrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muhammadluth/goslogx"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestPluginAndBusinessMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	logger := goslogx.New(goslogx.WithOutput(io.Discard), goslogx.WithMeterProvider(mp))
	h, err := NewHandler(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) }))
	if err != nil {
		t.Fatal(err)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "http://localhost/orders/SECRET?token=SECRET", nil))
	var data metricdata.ResourceMetrics
	if err = reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprint(data)
	for _, name := range []string{"orders.created", "http.server.request.duration", "/orders/{id}"} {
		if !strings.Contains(text, name) {
			t.Fatal("missing metric", name)
		}
	}
	if strings.Contains(text, "SECRET") {
		t.Fatal("sensitive metric")
	}
}
