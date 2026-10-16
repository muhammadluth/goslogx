package goslogx

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	resourceimpl "github.com/muhammadluth/goslogx/internal/telemetry/resource"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/proto"
)

func sharedTestResource() ResourceConfig {
	return ResourceConfig{ServiceVersion: "2.0", DeploymentEnvironment: "test", ServiceInstanceID: "instance"}
}

func TestSharedResourcePrecedence(t *testing.T) {
	cfg := completeResourceConfig()
	shared := sharedTestResource()
	shared.ResourceAttributes = map[string]string{"team.name": "core", "region": "local"}
	WithResource(shared)(cfg)
	shared.ResourceAttributes["region"] = "mutated"
	if got := resourceAttribute(t, cfg, attribute.Key("service.version")); got != "2.0" {
		t.Fatalf("version = %q", got)
	}
	if got := resourceAttribute(t, cfg, attribute.Key("team.name")); got != "core" {
		t.Fatalf("team = %q", got)
	}
	if got := resourceAttribute(t, cfg, attribute.Key("region")); got != "local" {
		t.Fatalf("region = %q", got)
	}
	cfg.Resource.ResourceAttributes["service.name"] = "override"
	if err := resourceimpl.Validate(cfg.ServiceName, resourceimpl.Resolve(resourceimpl.Config(cfg.Resource), resourceimpl.Config{ServiceVersion: cfg.Tracing.ServiceVersion, DeploymentEnvironment: cfg.Tracing.DeploymentEnvironment, ServiceInstanceID: cfg.Tracing.ServiceInstanceID, ResourceAttributes: cfg.Tracing.ResourceAttributes})); err == nil {
		t.Fatal("reserved attribute accepted")
	}
}

func TestManagedMetricsExport(t *testing.T) {
	receiver := startOTLPMetricsTestReceiver(t, false)
	logger := setupLog(WithOutput(&discardWriter{}), WithServiceName("core-test"), WithResource(sharedTestResource()), WithMetrics(MetricsConfig{Endpoint: receiver.endpoint(), Insecure: true, ExportInterval: time.Hour}))
	if err := logger.MetricsError(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Shutdown(context.Background()) })
	counter, err := logger.Meter("core-test").Int64Counter("core.requests")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(context.Background(), 3)
	if err := logger.ForceFlush(nil); err != nil {
		t.Fatal(err)
	}
	request := receiver.receive(t)
	if got := otlpAttribute(request.ResourceMetrics[0].Resource.Attributes, "service.version"); got != "2.0" {
		t.Fatalf("version = %q", got)
	}
	points := findOTLPMetric(t, otlpMetrics(request.ResourceMetrics), "core.requests").GetSum().GetDataPoints()
	if len(points) != 1 || points[0].GetAsInt() != 3 {
		t.Fatalf("points = %v", points)
	}
	if err := logger.Shutdown(nil); err != nil {
		t.Fatal(err)
	}
	if err := logger.ForceFlush(nil); !errors.Is(err, ErrMetricsShutdown) {
		t.Fatalf("flush after shutdown = %v", err)
	}
}

func TestManagedMetricsValidation(t *testing.T) {
	for _, cfg := range []MetricsConfig{
		{}, {Endpoint: "host:4317", ExportInterval: -1},
		{Endpoint: "host:4317", ExportTimeout: -1},
		{Endpoint: "http://host:4317"},
		{Endpoint: "host:4317", Headers: map[string]string{"Authorization": "secret"}},
		{Endpoint: "host:4317", Compression: "invalid"},
	} {
		logger := setupLog(WithOutput(&discardWriter{}), WithServiceName("core-test"), WithResource(sharedTestResource()), WithMetrics(cfg))
		if logger.MetricsError() == nil {
			t.Fatalf("accepted config: %+v", cfg)
		}
		if logger.telemetry.Lifecycle.MeterProvider != nil {
			t.Fatal("invalid config created provider")
		}
	}
	for _, reversed := range []bool{false, true} {
		options := []Option{WithMetrics(MetricsConfig{Endpoint: "host:4317"}), WithMeterProvider(noop.NewMeterProvider())}
		if reversed {
			options[0], options[1] = options[1], options[0]
		}
		logger := setupLog(append(options, WithOutput(&discardWriter{}))...)
		if logger.MetricsError() == nil {
			t.Fatal("ownership conflict accepted")
		}
	}
}

func TestMetricsGlobalsOptIn(t *testing.T) {
	receiver := startOTLPMetricsTestReceiver(t, false)
	previous := otel.GetMeterProvider()
	marker := noop.NewMeterProvider()
	otel.SetMeterProvider(marker)
	t.Cleanup(func() { otel.SetMeterProvider(previous) })
	for _, register := range []bool{false, true} {
		logger := setupLog(WithOutput(&discardWriter{}), WithServiceName("core-test"), WithResource(sharedTestResource()), WithMetrics(MetricsConfig{Endpoint: receiver.endpoint(), Insecure: true, RegisterGlobal: register}))
		if err := logger.MetricsError(); err != nil {
			t.Fatal(err)
		}
		if register && otel.GetMeterProvider() != logger.MeterProvider() {
			t.Fatal("global not installed")
		}
		if !register && otel.GetMeterProvider() != marker {
			t.Fatal("global changed without opt-in")
		}
		if err := logger.Shutdown(nil); err != nil {
			t.Fatal(err)
		}
		if otel.GetMeterProvider() != marker {
			t.Fatal("global not restored")
		}
	}
}

func TestUnifiedLifecycleConcurrent(t *testing.T) {
	metrics := &lifecycleMetricExporter{}
	spans := &ownershipSpanExporter{}
	logger := setupLog(WithOutput(&discardWriter{}))
	logger.telemetry.Lifecycle.MeterProvider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metrics, sdkmetric.WithInterval(time.Hour))))
	logger.telemetry.Lifecycle.TracerProvider = sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans))
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := logger.ForceFlush(nil)
			if err != nil && !errors.Is(err, ErrTracingShutdown) {
				t.Errorf("flush = %v", err)
			}
			if err := logger.Shutdown(nil); err != nil {
				t.Errorf("shutdown = %v", err)
			}
		}()
	}
	wg.Wait()
	if metrics.shutdown.Load() != 1 || spans.shutdownCall.Load() != 1 {
		t.Fatal("providers not shut down exactly once")
	}
}

type failingLifecycleMetricExporter struct {
	lifecycleMetricExporter
	err error
}

func (e *failingLifecycleMetricExporter) ForceFlush(context.Context) error { return e.err }
func (e *failingLifecycleMetricExporter) Shutdown(context.Context) error   { return e.err }

type failingLifecycleSpanExporter struct {
	ownershipSpanExporter
	err error
}

func (e *failingLifecycleSpanExporter) Shutdown(context.Context) error { return e.err }

func TestUnifiedLifecycleJoinsErrors(t *testing.T) {
	traceErr := errors.New("trace shutdown")
	metricErr := errors.New("metric shutdown")
	logger := setupLog(WithOutput(&discardWriter{}))
	logger.telemetry.Lifecycle.TracerProvider = sdktrace.NewTracerProvider(sdktrace.WithSyncer(&failingLifecycleSpanExporter{err: traceErr}))
	logger.telemetry.Lifecycle.MeterProvider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(&failingLifecycleMetricExporter{err: metricErr}, sdkmetric.WithInterval(time.Hour))))
	if err := logger.ForceFlush(nil); !errors.Is(err, metricErr) {
		t.Fatalf("flush = %v", err)
	}
	for range 2 {
		if err := logger.Shutdown(nil); !errors.Is(err, traceErr) || !errors.Is(err, metricErr) {
			t.Fatalf("shutdown = %v", err)
		}
	}
}

func TestTelemetryAPIs(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	defer provider.Shutdown(context.Background())
	logger := setupLog(WithOutput(&discardWriter{}), WithTracerProvider(provider))
	ctx, span := logger.StartSpan(nil, "operation", trace.WithSpanKind(trace.SpanKindClient))
	if !trace.SpanContextFromContext(ctx).IsValid() {
		t.Fatal("span context missing")
	}
	span.End()
	var nilLogger *Logger
	_, span = nilLogger.StartSpan(nil, "noop")
	if span.IsRecording() {
		t.Fatal("nil logger recorded a span")
	}
	span.End()
	if nilLogger.Meter("") == nil || nilLogger.Tracer("") == nil {
		t.Fatal("nil logger returned nil instrumentation")
	}
	old := globalLog.Load()
	globalLog.Store(logger)
	defer globalLog.Store(old)
	if Meter("") == nil || Tracer("") == nil || MetricsError() != nil {
		t.Fatal("global APIs failed")
	}
	_, span = StartSpan(nil, "global")
	if !span.SpanContext().IsValid() {
		t.Fatal("global span context missing")
	}
	span.End()
}

func TestLifecycleWaitRespectsContext(t *testing.T) {
	logger := setupLog(WithOutput(&discardWriter{}))
	logger.telemetry.Lifecycle.MeterProvider = sdkmetric.NewMeterProvider()
	defer logger.telemetry.Lifecycle.MeterProvider.Shutdown(context.Background())
	logger.telemetry.Lifecycle.Gate <- struct{}{}
	defer func() { <-logger.telemetry.Lifecycle.Gate }()
	for _, operation := range []func(context.Context) error{logger.ForceFlush, logger.Shutdown} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		err := operation(ctx)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("wait = %v", err)
		}
	}
	if logger.telemetry.Lifecycle.Closed.Load() {
		t.Fatal("waiting caller closed telemetry")
	}
}

func TestManagedResourceIdentityAcrossSignals(t *testing.T) {
	traces := startOTLPTestReceiver(t, false)
	metrics := startOTLPMetricsTestReceiver(t, false)
	cfg := integrationTracingConfig(traces.endpoint())
	cfg.ServiceVersion = "legacy"
	logger := setupLog(WithOutput(&discardWriter{}), WithServiceName("payment-api"), WithResource(sharedTestResource()), WithTracing(cfg), WithMetrics(MetricsConfig{Endpoint: metrics.endpoint(), Insecure: true, ExportInterval: time.Hour}))
	if err := errors.Join(logger.TracingError(), logger.MetricsError()); err != nil {
		t.Fatal(err)
	}
	defer logger.Shutdown(context.Background())
	_, span := logger.StartSpan(context.Background(), "payment.process", trace.WithAttributes(attribute.String("payment.partner", "partner-a")))
	span.End()
	counter, err := logger.Meter("payment").Int64Counter("payment.transactions")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(context.Background(), 1)
	if err := logger.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	traceRequest := traces.receive(t)
	metricRequest := metrics.receive(t)
	tr, mr := traceRequest.ResourceSpans[0], metricRequest.ResourceMetrics[0]
	if tr.SchemaUrl != mr.SchemaUrl {
		t.Fatalf("schema mismatch %q %q", tr.SchemaUrl, mr.SchemaUrl)
	}
	if !proto.Equal(tr.Resource, mr.Resource) {
		t.Fatalf("resource mismatch: %v %v", tr.Resource, mr.Resource)
	}
	if got := otlpAttribute(tr.Resource.Attributes, "service.version"); got != "2.0" {
		t.Fatal(got)
	}
}

func TestNativeSpanOptionsAndCustomScope(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())
	logger := setupLog(WithOutput(&discardWriter{}), WithTracerProvider(tp))
	started := time.Now().Add(-time.Second)
	link := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}})
	_, span := logger.Tracer("payment", trace.WithInstrumentationVersion("2.0")).Start(context.Background(), "payment.process", trace.WithTimestamp(started), trace.WithLinks(trace.Link{SpanContext: link}), trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(attribute.String("payment.partner", "partner-a")))
	span.AddEvent("validated")
	span.SetStatus(codes.Ok, "")
	span.End()
	ended := recorder.Ended()[0]
	if ended.InstrumentationScope().Name != "payment" || ended.InstrumentationScope().Version != "2.0" || ended.SpanKind() != trace.SpanKindProducer || !ended.StartTime().Equal(started) || len(ended.Links()) != 1 || len(ended.Events()) != 1 || len(ended.Attributes()) != 1 || ended.Status().Code != codes.Ok {
		t.Fatalf("native options lost: %v", ended)
	}
}

func TestDisabledSpanPreservesParent(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	ctx, parent := tp.Tracer("parent").Start(context.Background(), "parent")
	defer parent.End()
	logger := setupLog(WithOutput(&discardWriter{}))
	childCtx, span := logger.StartSpan(ctx, "disabled")
	span.End()
	if childCtx != ctx || !parent.IsRecording() || span.IsRecording() {
		t.Fatal("disabled span changed parent")
	}
	if allocs := testing.AllocsPerRun(100, func() { _, span := logger.StartSpan(ctx, "disabled"); span.End() }); allocs != 0 {
		t.Fatalf("disabled span allocations: %f", allocs)
	}
}

func TestNewOptionsSnapshotAndReuse(t *testing.T) {
	headers := map[string]string{"Authorization": "initial"}
	attrs := map[string]string{"team.name": "initial"}
	metricsOption := WithMetrics(MetricsConfig{Headers: headers})
	resourceOption := WithResource(ResourceConfig{ResourceAttributes: attrs})
	headers["Authorization"] = "changed"
	attrs["team.name"] = "changed"
	first, second := defaultConfig(), defaultConfig()
	metricsOption(first)
	resourceOption(first)
	first.Metrics.Headers["Authorization"] = "mutated"
	first.Resource.ResourceAttributes["team.name"] = "mutated"
	metricsOption(second)
	resourceOption(second)
	if second.Metrics.Headers["Authorization"] != "initial" || second.Resource.ResourceAttributes["team.name"] != "initial" {
		t.Fatal("option snapshots share mutable configuration")
	}
}

func TestManagedMetricsLifecycleNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	receiver := startOTLPMetricsTestReceiver(t, false)
	logger := setupLog(WithOutput(&discardWriter{}), WithServiceName("payment-api"), WithResource(sharedTestResource()), WithMetrics(MetricsConfig{Endpoint: receiver.endpoint(), Insecure: true, ExportInterval: time.Hour}))
	if err := logger.MetricsError(); err != nil {
		t.Fatal(err)
	}
	defer logger.Shutdown(context.Background())
	counter, err := logger.Meter("payment").Int64Counter("payment.transactions")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(context.Background(), 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := logger.ForceFlush(ctx); err != nil {
		t.Fatal(err)
	}
	_ = receiver.receive(t)
	if err := logger.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	receiver.stop()
}

func TestShutdownPreservesReplacedPropagator(t *testing.T) {
	receiver := startOTLPTestReceiver(t, false)
	original := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(original)
	cfg := integrationTracingConfig(receiver.endpoint())
	cfg.RegisterGlobal = true
	logger := setupLog(WithOutput(&discardWriter{}), WithServiceName("payment-api"), WithTracing(cfg))
	if err := logger.TracingError(); err != nil {
		t.Fatal(err)
	}
	// Composite propagators contain a slice: comparing two values can panic.
	marker := propagation.NewCompositeTextMapPropagator(propagation.Baggage{})
	otel.SetTextMapPropagator(marker)
	if err := logger.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	fields := otel.GetTextMapPropagator().Fields()
	if len(fields) != 1 || fields[0] != "baggage" {
		t.Fatalf("replacement overwritten: %v", fields)
	}
}

func TestExternalTracingRemainsUsableAfterManagedMetricsShutdown(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())
	logger := setupLog(WithOutput(&discardWriter{}), WithTracerProvider(tp))
	logger.telemetry.Lifecycle.MeterProvider = sdkmetric.NewMeterProvider()
	if err := logger.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, end := logger.StartTrace(context.Background(), "external.after.metrics")
	end()
	if len(recorder.Ended()) != 1 {
		t.Fatal("managed metrics shutdown disabled external tracing")
	}
}
