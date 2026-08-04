package goslogx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type exportedSpan struct {
	name   string
	kind   trace.SpanKind
	span   trace.SpanContext
	parent trace.SpanContext
}

type recordingSpanExporter struct {
	mu    sync.Mutex
	spans []exportedSpan
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func (e *recordingSpanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, span := range spans {
		e.spans = append(e.spans, exportedSpan{
			name:   span.Name(),
			kind:   span.SpanKind(),
			span:   span.SpanContext(),
			parent: span.Parent(),
		})
	}
	return nil
}

func (*recordingSpanExporter) Shutdown(context.Context) error { return nil }

func (e *recordingSpanExporter) snapshot() []exportedSpan {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]exportedSpan(nil), e.spans...)
}

func TestHTTPInstrumentationPropagatesAndCorrelatesLogs(t *testing.T) {
	exporter := &recordingSpanExporter{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})

	var logOutput bytes.Buffer
	logger := setupLog(WithOutput(&logOutput))
	var downstreamTraceparent string
	client := &http.Client{Transport: otelhttp.NewTransport(roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		downstreamTraceparent = request.Header.Get("traceparent")
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	}))}
	upstream := otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.InfoContext(r.Context(), "request-123", "payment-api", MESSSAGE_TYPE_EVENT, "request received", nil)

		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://inventory.internal/items", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response, err := client.Do(request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		w.WriteHeader(http.StatusNoContent)
	}), "payment-api")

	request := httptest.NewRequest(http.MethodGet, "http://payment.internal/payments", nil)
	request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	response := httptest.NewRecorder()
	upstream.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("upstream status = %d, want %d", response.Code, http.StatusNoContent)
	}

	if downstreamTraceparent == "" {
		t.Fatal("outbound request did not carry traceparent")
	}

	spans := exporter.snapshot()
	serverSpan := findExportedSpan(t, spans, "payment-api", trace.SpanKindServer)
	outboundSpan := findChildSpan(t, spans, serverSpan.span, trace.SpanKindClient)
	if outboundSpan.span.TraceID() != serverSpan.span.TraceID() {
		t.Errorf("outbound trace ID = %s, want %s", outboundSpan.span.TraceID(), serverSpan.span.TraceID())
	}
	traceparentParts := strings.Split(downstreamTraceparent, "-")
	if len(traceparentParts) != 4 || traceparentParts[1] != outboundSpan.span.TraceID().String() || traceparentParts[2] != outboundSpan.span.SpanID().String() {
		t.Errorf("propagated traceparent = %q, want trace %s and span %s", downstreamTraceparent, outboundSpan.span.TraceID(), outboundSpan.span.SpanID())
	}

	var logged map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logOutput.Bytes()), &logged); err != nil {
		t.Fatalf("decode correlated log: %v; output: %s", err, logOutput.String())
	}
	if got := logged["obs_trace_id"]; got != serverSpan.span.TraceID().String() {
		t.Errorf("log obs_trace_id = %v, want %s", got, serverSpan.span.TraceID())
	}
}

func TestGRPCInstrumentationBoundaryConfiguration(t *testing.T) {
	server := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	server.Stop()

	connection, err := grpc.NewClient(
		"passthrough:///unused",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		t.Fatalf("configure instrumented gRPC client: %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close gRPC client: %v", err)
	}
}

func findChildSpan(t *testing.T, spans []exportedSpan, parent trace.SpanContext, kind trace.SpanKind) exportedSpan {
	t.Helper()

	for _, span := range spans {
		if span.kind == kind && span.parent.SpanID() == parent.SpanID() {
			return span
		}
	}
	t.Fatalf("child span with kind %s and parent %s not found in %#v", kind, parent.SpanID(), spans)
	return exportedSpan{}
}

func findExportedSpan(t *testing.T, spans []exportedSpan, name string, kind trace.SpanKind) exportedSpan {
	t.Helper()

	for _, span := range spans {
		if span.name == name && span.kind == kind {
			return span
		}
	}
	t.Fatalf("span %q with kind %s not found in %#v", name, kind, spans)
	return exportedSpan{}
}
