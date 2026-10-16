package goslogx

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	collecttrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
)

type otlpTestReceiver struct {
	collecttrace.UnimplementedTraceServiceServer

	listener net.Listener
	server   *grpc.Server
	requests chan *collecttrace.ExportTraceServiceRequest
	block    bool
}

func startOTLPTestReceiver(t *testing.T, block bool) *otlpTestReceiver {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for OTLP test receiver: %v", err)
	}
	receiver := &otlpTestReceiver{
		listener: listener,
		server:   grpc.NewServer(),
		requests: make(chan *collecttrace.ExportTraceServiceRequest, 8),
		block:    block,
	}
	collecttrace.RegisterTraceServiceServer(receiver.server, receiver)
	go func() { _ = receiver.server.Serve(listener) }()
	t.Cleanup(receiver.stop)
	return receiver
}

func (r *otlpTestReceiver) Export(ctx context.Context, request *collecttrace.ExportTraceServiceRequest) (*collecttrace.ExportTraceServiceResponse, error) {
	if r.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	select {
	case r.requests <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &collecttrace.ExportTraceServiceResponse{}, nil
}

func (r *otlpTestReceiver) endpoint() string { return r.listener.Addr().String() }

func (r *otlpTestReceiver) stop() {
	if r.server != nil {
		r.server.Stop()
	}
}

func (r *otlpTestReceiver) receive(t *testing.T) *collecttrace.ExportTraceServiceRequest {
	t.Helper()

	select {
	case request := <-r.requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("OTLP receiver did not receive an export request")
		return nil
	}
}

func TestOTLPExportEndToEnd(t *testing.T) {
	receiver := startOTLPTestReceiver(t, false)
	config := integrationTracingConfig(receiver.endpoint())
	config.BatchTimeout = time.Hour
	logger := setupLog(
		WithOutput(&discardWriter{}),
		WithServiceName("payment-api"),
		WithTracing(config),
	)
	if err := logger.TracingError(); err != nil {
		t.Fatalf("TracingError() = %v", err)
	}

	rootCtx, endRoot := logger.StartTrace(context.Background(), "payment.root")
	rootSpan := trace.SpanFromContext(rootCtx)
	rootSpan.SetAttributes(attribute.String("payment.id", "pay-123"))
	rootSpanContext := rootSpan.SpanContext()

	header := make(http.Header)
	InjectHTTPContext(rootCtx, header)
	propagatedCtx := ExtractHTTPContext(context.Background(), header)
	childCtx, endChild := logger.StartTrace(propagatedCtx, "payment.child")
	childSpanContext := trace.SpanContextFromContext(childCtx)
	logger.ErrorContext(childCtx, "request-123", "payment", errors.New("payment rejected"))
	endChild()
	endRoot()

	select {
	case <-receiver.requests:
		t.Fatal("batch exported before ForceFlush")
	default:
	}
	flushCtx, cancelFlush := context.WithTimeout(context.Background(), time.Second)
	defer cancelFlush()
	if err := logger.ForceFlush(flushCtx); err != nil {
		t.Fatalf("ForceFlush() error = %v", err)
	}

	request := receiver.receive(t)
	resourceSpans := request.GetResourceSpans()
	if len(resourceSpans) != 1 {
		t.Fatalf("resource spans = %d, want 1", len(resourceSpans))
	}
	if got := otlpAttribute(resourceSpans[0].GetResource().GetAttributes(), "service.name"); got != "payment-api" {
		t.Errorf("service.name = %q, want payment-api", got)
	}
	spans := otlpSpans(resourceSpans)
	root := findOTLPSpan(t, spans, "payment.root")
	child := findOTLPSpan(t, spans, "payment.child")
	if got := hex.EncodeToString(root.GetTraceId()); got != rootSpanContext.TraceID().String() {
		t.Errorf("root trace ID = %s, want %s", got, rootSpanContext.TraceID())
	}
	if got := hex.EncodeToString(child.GetTraceId()); got != childSpanContext.TraceID().String() {
		t.Errorf("child trace ID = %s, want %s", got, childSpanContext.TraceID())
	}
	if got := hex.EncodeToString(child.GetParentSpanId()); got != rootSpanContext.SpanID().String() {
		t.Errorf("child parent span ID = %s, want %s", got, rootSpanContext.SpanID())
	}
	if got := otlpAttribute(root.GetAttributes(), "payment.id"); got != "pay-123" {
		t.Errorf("payment.id = %q, want pay-123", got)
	}
	if child.GetStatus().GetCode() != tracepb.Status_STATUS_CODE_ERROR {
		t.Errorf("child status = %s, want ERROR", child.GetStatus().GetCode())
	}
	if root.GetFlags()&1 == 0 || child.GetFlags()&1 == 0 {
		t.Error("sampling flag was not propagated to root and child spans")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if err := logger.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := logger.ForceFlush(context.Background()); !errors.Is(err, ErrTracingShutdown) {
		t.Errorf("ForceFlush() after shutdown = %v, want ErrTracingShutdown", err)
	}
}

func TestOTLPExportFailures(t *testing.T) {
	t.Run("endpoint unavailable", func(t *testing.T) {
		endpoint := closedLoopbackEndpoint(t)
		logger := newIntegrationLogger(t, integrationTracingConfig(endpoint))
		endIntegrationSpan(logger)
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		if err := logger.ForceFlush(ctx); err == nil {
			t.Fatal("ForceFlush() error = nil, want unavailable endpoint error")
		}
	})

	t.Run("receiver stops before export", func(t *testing.T) {
		receiver := startOTLPTestReceiver(t, false)
		logger := newIntegrationLogger(t, integrationTracingConfig(receiver.endpoint()))
		receiver.stop()
		endIntegrationSpan(logger)
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		if err := logger.ForceFlush(ctx); err == nil {
			t.Fatal("ForceFlush() error = nil after receiver stopped")
		}
	})

	t.Run("export timeout", func(t *testing.T) {
		receiver := startOTLPTestReceiver(t, true)
		config := integrationTracingConfig(receiver.endpoint())
		config.ExportTimeout = 20 * time.Millisecond
		logger := newIntegrationLogger(t, config)
		endIntegrationSpan(logger)
		if err := logger.ForceFlush(context.Background()); err == nil {
			t.Fatal("ForceFlush() error = nil, want export timeout")
		}
	})

	t.Run("shutdown timeout", func(t *testing.T) {
		receiver := startOTLPTestReceiver(t, true)
		logger := newIntegrationLogger(t, integrationTracingConfig(receiver.endpoint()))
		endIntegrationSpan(logger)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if err := logger.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Shutdown() error = %v, want context deadline exceeded", err)
		}
	})
}

func TestOTLPGracefulShutdownFlushesPendingSpan(t *testing.T) {
	receiver := startOTLPTestReceiver(t, false)
	config := integrationTracingConfig(receiver.endpoint())
	config.BatchTimeout = time.Hour
	logger := newIntegrationLogger(t, config)
	endIntegrationSpan(logger)

	select {
	case <-receiver.requests:
		t.Fatal("batch exported before graceful shutdown")
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := logger.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	request := receiver.receive(t)
	if span := findOTLPSpan(t, otlpSpans(request.GetResourceSpans()), "failure.span"); span == nil {
		t.Fatal("pending span was not delivered during graceful shutdown")
	}
}

func TestOTLPLifecycleDoesNotLeakGoroutines(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	receiver := startOTLPTestReceiver(t, false)
	logger := newIntegrationLogger(t, integrationTracingConfig(receiver.endpoint()))
	endIntegrationSpan(logger)
	flushCtx, cancelFlush := context.WithTimeout(context.Background(), time.Second)
	if err := logger.ForceFlush(flushCtx); err != nil {
		cancelFlush()
		t.Fatalf("ForceFlush() error = %v", err)
	}
	cancelFlush()
	_ = receiver.receive(t)

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	if err := logger.Shutdown(shutdownCtx); err != nil {
		cancelShutdown()
		t.Fatalf("Shutdown() error = %v", err)
	}
	cancelShutdown()
	receiver.stop()
}

func integrationTracingConfig(endpoint string) TracingConfig {
	return TracingConfig{
		Endpoint:              endpoint,
		Insecure:              true,
		SamplingRatio:         1,
		ServiceVersion:        "1.0.0-test",
		DeploymentEnvironment: "integration-test",
		ServiceInstanceID:     "integration-instance",
	}
}

func newIntegrationLogger(t *testing.T, config TracingConfig) *Logger {
	t.Helper()

	logger := setupLog(
		WithOutput(&discardWriter{}),
		WithServiceName("payment-api"),
		WithTracing(config),
	)
	if err := logger.TracingError(); err != nil {
		t.Fatalf("TracingError() = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = logger.Shutdown(ctx)
	})
	return logger
}

func endIntegrationSpan(logger *Logger) {
	_, end := logger.StartTrace(context.Background(), "failure.span")
	end()
}

func closedLoopbackEndpoint(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve unavailable endpoint: %v", err)
	}
	endpoint := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close reserved endpoint: %v", err)
	}
	return endpoint
}

func otlpSpans(resources []*tracepb.ResourceSpans) []*tracepb.Span {
	var spans []*tracepb.Span
	for _, resourceSpans := range resources {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			spans = append(spans, scopeSpans.GetSpans()...)
		}
	}
	return spans
}

func findOTLPSpan(t *testing.T, spans []*tracepb.Span, name string) *tracepb.Span {
	t.Helper()

	for _, span := range spans {
		if span.GetName() == name {
			return span
		}
	}
	t.Fatalf("OTLP span %q not found", name)
	return nil
}

func otlpAttribute(attributes []*commonpb.KeyValue, key string) string {
	for _, item := range attributes {
		if item.GetKey() == key {
			return item.GetValue().GetStringValue()
		}
	}
	return ""
}
