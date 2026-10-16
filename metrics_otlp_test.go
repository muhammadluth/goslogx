package goslogx

import (
	"context"
	"encoding/hex"
	"net"
	"testing"
	"time"

	otelattribute "go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	"go.opentelemetry.io/otel/trace"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
)

type otlpMetricsTestReceiver struct {
	collectmetrics.UnimplementedMetricsServiceServer

	listener net.Listener
	server   *grpc.Server
	requests chan *collectmetrics.ExportMetricsServiceRequest
	block    bool
}

func startOTLPMetricsTestReceiver(t *testing.T, block bool) *otlpMetricsTestReceiver {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for OTLP metrics test receiver: %v", err)
	}
	receiver := &otlpMetricsTestReceiver{
		listener: listener,
		server:   grpc.NewServer(),
		requests: make(chan *collectmetrics.ExportMetricsServiceRequest, 8),
		block:    block,
	}
	collectmetrics.RegisterMetricsServiceServer(receiver.server, receiver)
	go func() { _ = receiver.server.Serve(listener) }()
	t.Cleanup(receiver.stop)
	return receiver
}

func (r *otlpMetricsTestReceiver) Export(ctx context.Context, request *collectmetrics.ExportMetricsServiceRequest) (*collectmetrics.ExportMetricsServiceResponse, error) {
	if r.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	select {
	case r.requests <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &collectmetrics.ExportMetricsServiceResponse{}, nil
}

func (r *otlpMetricsTestReceiver) endpoint() string { return r.listener.Addr().String() }

func (r *otlpMetricsTestReceiver) stop() {
	if r.server != nil {
		r.server.Stop()
	}
}

func (r *otlpMetricsTestReceiver) receive(t *testing.T) *collectmetrics.ExportMetricsServiceRequest {
	t.Helper()

	select {
	case request := <-r.requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("OTLP metrics receiver did not receive an export request")
		return nil
	}
}

func TestOTLPMetricsExportEndToEnd(t *testing.T) {
	receiver := startOTLPMetricsTestReceiver(t, false)
	provider := newOTLPMetricsProvider(t, receiver.endpoint(), time.Second)
	logger := setupLog(
		WithOutput(&discardWriter{}),
		WithServiceName("payment-api"),
		WithMeterProvider(provider),
	)
	if logger.MeterProvider() != provider {
		t.Fatal("goslogx did not retain the application MeterProvider")
	}

	meter := logger.MeterProvider().Meter("payment-api")
	counter, err := meter.Int64Counter("payment.requests")
	if err != nil {
		t.Fatalf("create counter: %v", err)
	}
	histogram, err := meter.Float64Histogram("payment.processing.duration", metric.WithUnit("s"))
	if err != nil {
		t.Fatalf("create histogram: %v", err)
	}

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
	recordContext := trace.ContextWithSpanContext(context.Background(), spanContext)
	attributes := metric.WithAttributes(
		otelattribute.String("http.request.method", "POST"),
		otelattribute.String("http.route", "/payments/{id}"),
	)
	counter.Add(recordContext, 7, attributes)
	histogram.Record(recordContext, 0.25, attributes)
	histogram.Record(recordContext, 0.75, attributes)

	if err := logger.ForceFlush(context.Background()); err != nil {
		t.Fatalf("goslogx ForceFlush: %v", err)
	}
	select {
	case <-receiver.requests:
		t.Fatal("goslogx flushed an application-owned MeterProvider")
	default:
	}

	flushContext, cancelFlush := context.WithTimeout(context.Background(), time.Second)
	defer cancelFlush()
	if err := provider.ForceFlush(flushContext); err != nil {
		t.Fatalf("application MeterProvider ForceFlush: %v", err)
	}

	request := receiver.receive(t)
	resourceMetrics := request.GetResourceMetrics()
	if len(resourceMetrics) != 1 {
		t.Fatalf("resource metrics = %d, want 1", len(resourceMetrics))
	}
	if got := otlpAttribute(resourceMetrics[0].GetResource().GetAttributes(), "service.name"); got != "payment-api" {
		t.Errorf("service.name = %q, want payment-api", got)
	}

	metrics := otlpMetrics(resourceMetrics)
	counterMetric := findOTLPMetric(t, metrics, "payment.requests")
	counterPoints := counterMetric.GetSum().GetDataPoints()
	if len(counterPoints) != 1 {
		t.Fatalf("counter data points = %d, want 1", len(counterPoints))
	}
	if got := counterPoints[0].GetAsInt(); got != 7 {
		t.Errorf("counter value = %d, want 7", got)
	}
	assertOTLPMetricAttributes(t, counterPoints[0].GetAttributes())

	histogramMetric := findOTLPMetric(t, metrics, "payment.processing.duration")
	histogramPoints := histogramMetric.GetHistogram().GetDataPoints()
	if len(histogramPoints) != 1 {
		t.Fatalf("histogram data points = %d, want 1", len(histogramPoints))
	}
	point := histogramPoints[0]
	if got := point.GetCount(); got != 2 {
		t.Errorf("histogram count = %d, want 2", got)
	}
	if got := point.GetSum(); got != 1 {
		t.Errorf("histogram sum = %f, want 1", got)
	}
	assertOTLPMetricAttributes(t, point.GetAttributes())
	assertOTLPExemplar(t, point.GetExemplars(), spanContext)

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if err := provider.Shutdown(shutdownContext); err != nil {
		t.Fatalf("application MeterProvider Shutdown: %v", err)
	}
}

func TestOTLPMetricsShutdownFlushesPendingData(t *testing.T) {
	receiver := startOTLPMetricsTestReceiver(t, false)
	provider := newOTLPMetricsProvider(t, receiver.endpoint(), time.Second)
	counter, err := provider.Meter("shutdown-test").Int64Counter("shutdown.pending")
	if err != nil {
		t.Fatalf("create counter: %v", err)
	}
	counter.Add(context.Background(), 1)

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if err := provider.Shutdown(shutdownContext); err != nil {
		t.Fatalf("MeterProvider Shutdown: %v", err)
	}
	if metric := findOTLPMetric(t, otlpMetrics(receiver.receive(t).GetResourceMetrics()), "shutdown.pending"); metric == nil {
		t.Fatal("pending metric was not exported during shutdown")
	}
}

func TestOTLPMetricsExportFailures(t *testing.T) {
	t.Run("endpoint unavailable", func(t *testing.T) {
		provider := newOTLPMetricsProvider(t, closedLoopbackEndpoint(t), 75*time.Millisecond)
		counter, err := provider.Meter("failure-test").Int64Counter("failure.counter")
		if err != nil {
			t.Fatalf("create counter: %v", err)
		}
		counter.Add(context.Background(), 1)

		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		if err := provider.ForceFlush(ctx); err == nil {
			t.Fatal("ForceFlush error = nil, want unavailable endpoint error")
		}
	})

	t.Run("export timeout", func(t *testing.T) {
		receiver := startOTLPMetricsTestReceiver(t, true)
		provider := newOTLPMetricsProvider(t, receiver.endpoint(), 20*time.Millisecond)
		counter, err := provider.Meter("failure-test").Int64Counter("failure.counter")
		if err != nil {
			t.Fatalf("create counter: %v", err)
		}
		counter.Add(context.Background(), 1)

		if err := provider.ForceFlush(context.Background()); err == nil {
			t.Fatal("ForceFlush error = nil, want export timeout")
		}
	})
}

func newOTLPMetricsProvider(t *testing.T, endpoint string, exportTimeout time.Duration) *sdkmetric.MeterProvider {
	t.Helper()

	exporter, err := otlpmetricgrpc.New(
		context.Background(),
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithTimeout(exportTimeout),
		otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig{Enabled: false}),
	)
	if err != nil {
		t.Fatalf("create OTLP metrics exporter: %v", err)
	}
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(resource.NewSchemaless(semconv.ServiceName("payment-api"))),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			exporter,
			sdkmetric.WithInterval(time.Hour),
		)),
	)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = provider.Shutdown(ctx)
	})
	return provider
}

func otlpMetrics(resources []*metricspb.ResourceMetrics) []*metricspb.Metric {
	var metrics []*metricspb.Metric
	for _, resourceMetrics := range resources {
		for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
			metrics = append(metrics, scopeMetrics.GetMetrics()...)
		}
	}
	return metrics
}

func findOTLPMetric(t *testing.T, metrics []*metricspb.Metric, name string) *metricspb.Metric {
	t.Helper()

	for _, current := range metrics {
		if current.GetName() == name {
			return current
		}
	}
	t.Fatalf("OTLP metric %q not found", name)
	return nil
}

func assertOTLPMetricAttributes(t *testing.T, attributes []*commonpb.KeyValue) {
	t.Helper()

	if got := otlpAttribute(attributes, "http.request.method"); got != "POST" {
		t.Errorf("http.request.method = %q, want POST", got)
	}
	if got := otlpAttribute(attributes, "http.route"); got != "/payments/{id}" {
		t.Errorf("http.route = %q, want /payments/{id}", got)
	}
}

func assertOTLPExemplar(t *testing.T, exemplars []*metricspb.Exemplar, spanContext trace.SpanContext) {
	t.Helper()

	if len(exemplars) == 0 {
		t.Fatal("histogram trace exemplar was not exported")
	}
	for _, exemplar := range exemplars {
		if hex.EncodeToString(exemplar.GetTraceId()) == spanContext.TraceID().String() &&
			hex.EncodeToString(exemplar.GetSpanId()) == spanContext.SpanID().String() {
			return
		}
	}
	t.Fatalf("trace exemplar does not reference trace %s span %s", spanContext.TraceID(), spanContext.SpanID())
}

var _ collectmetrics.MetricsServiceServer = (*otlpMetricsTestReceiver)(nil)
