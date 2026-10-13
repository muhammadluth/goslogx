package goslogx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/credentials"
)

const (
	instrumentationName = "github.com/muhammadluth/goslogx"

	// DefaultSamplingRatio samples all root traces unless configured otherwise.
	DefaultSamplingRatio = 1.0
	// DefaultBatchTimeout is the maximum delay before exporting queued spans.
	DefaultBatchTimeout = 5 * time.Second
	// DefaultExportTimeout bounds each export attempt.
	DefaultExportTimeout = 30 * time.Second
	// DefaultMaxQueueSize is the number of spans buffered before new spans drop.
	DefaultMaxQueueSize = 2048
	// DefaultMaxExportBatchSize is the maximum number of spans per export.
	DefaultMaxExportBatchSize = 512
	// DefaultOTelErrorLogInterval limits repeated internal telemetry errors.
	DefaultOTelErrorLogInterval = time.Minute
)

// ErrTracingShutdown indicates that the logger-owned tracing provider has
// already begun shutting down.
var ErrTracingShutdown = errors.New("goslogx: tracing provider is shut down")

var telemetryGlobalsMu sync.Mutex

// A pointer wrapper lets shutdown recognize its own global registration even
// when another application installs a non-comparable composite propagator.
type managedPropagator struct{ propagation.TextMapPropagator }

type batchProcessorConfig struct {
	BatchTimeout       time.Duration
	ExportTimeout      time.Duration
	MaxQueueSize       int
	MaxExportBatchSize int
}

type rateLimitedOTelErrorHandler struct {
	logger          *zap.Logger
	internalMetrics *internalMetrics
	interval        time.Duration
	mu              sync.Mutex
	lastLogged      time.Time
	suppressed      uint64
}

var reservedResourceAttributeKeys = map[string]struct{}{
	string(semconv.ServiceNameKey):               {},
	string(semconv.ServiceVersionKey):            {},
	string(semconv.ServiceInstanceIDKey):         {},
	string(semconv.DeploymentEnvironmentNameKey): {},
}

func noopEndTrace() {}

func setupTracing(cfg *Config) (trace.Tracer, *sdktrace.TracerProvider, error) {
	if cfg.TracerProvider != nil {
		if cfg.Tracing.Enabled {
			return nil, nil, fmt.Errorf("goslogx: WithTracing and WithTracerProvider cannot be combined")
		}
		return cfg.TracerProvider.Tracer(instrumentationName), nil, nil
	}
	if !cfg.Tracing.Enabled {
		return nil, nil, nil
	}
	if err := validateTracingConfig(cfg.Tracing); err != nil {
		return nil, nil, err
	}
	if err := validateResourceConfig(cfg); err != nil {
		return nil, nil, err
	}

	provider, err := newTracerProvider(context.Background(), cfg)
	if err != nil {
		return nil, nil, err
	}

	return provider.Tracer(instrumentationName), provider, nil
}

func validateTracingConfig(cfg TracingConfig) error {
	if !cfg.Enabled {
		return nil
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		return fmt.Errorf("goslogx: tracing endpoint is required")
	}
	if endpoint != strings.TrimSpace(endpoint) {
		return fmt.Errorf("goslogx: tracing endpoint must not contain surrounding whitespace")
	}
	if cfg.SamplingRatio < 0 || cfg.SamplingRatio > 1 {
		return fmt.Errorf("goslogx: tracing sampling ratio must be between 0.0 and 1.0")
	}
	if cfg.Insecure && cfg.TLSConfig != nil {
		return fmt.Errorf("goslogx: tracing TLS config cannot be combined with insecure transport")
	}
	if cfg.TLSConfig != nil && cfg.TLSConfig.MinVersion != 0 && cfg.TLSConfig.MinVersion < tls.VersionTLS12 {
		return fmt.Errorf("goslogx: tracing TLS minimum version must be TLS 1.2 or newer")
	}
	if cfg.Compression != "" && cfg.Compression != "gzip" {
		return fmt.Errorf("goslogx: tracing compression must be empty or gzip")
	}
	if cfg.ErrorLogInterval < 0 {
		return fmt.Errorf("goslogx: tracing error log interval must not be negative")
	}
	for key, value := range cfg.Headers {
		if !validOTLPHeaderKey(key) || !validOTLPHeaderValue(value) {
			return fmt.Errorf("goslogx: tracing headers contain invalid data")
		}
	}
	if _, err := resolveBatchProcessorConfig(cfg); err != nil {
		return err
	}

	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return fmt.Errorf("goslogx: invalid tracing endpoint")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("goslogx: tracing endpoint scheme must be http or https")
		}
		if parsed.Scheme == "http" && !cfg.Insecure {
			return fmt.Errorf("goslogx: HTTP tracing endpoint requires insecure transport")
		}
		if parsed.Scheme == "https" && cfg.Insecure {
			return fmt.Errorf("goslogx: HTTPS tracing endpoint cannot use insecure transport")
		}
		if parsed.Hostname() == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("goslogx: invalid tracing endpoint")
		}
		if port := parsed.Port(); port != "" {
			return validateTracingPort(port)
		}
		return nil
	}

	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || host == "" {
		return fmt.Errorf("goslogx: tracing endpoint must use host:port or http(s)://host:port")
	}
	return validateTracingPort(port)
}

func validOTLPHeaderKey(key string) bool {
	if key == "" || strings.HasPrefix(key, "grpc-") {
		return false
	}
	for i := 0; i < len(key); i++ {
		character := key[i]
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func validOTLPHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < 0x20 || value[i] > 0x7e {
			return false
		}
	}
	return true
}

func newTraceSampler(cfg TracingConfig) sdktrace.Sampler {
	ratio := cfg.SamplingRatio
	if ratio == 0 {
		ratio = DefaultSamplingRatio
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}

func newRateLimitedOTelErrorHandler(logger *zap.Logger, interval time.Duration, internalMetrics *internalMetrics) *rateLimitedOTelErrorHandler {
	if interval == 0 {
		interval = DefaultOTelErrorLogInterval
	}
	return &rateLimitedOTelErrorHandler{
		logger:          logger,
		internalMetrics: internalMetrics,
		interval:        interval,
	}
}

func (h *rateLimitedOTelErrorHandler) Handle(err error) {
	if h == nil || h.logger == nil || err == nil {
		return
	}
	h.internalMetrics.recordTelemetryError(context.Background())

	h.mu.Lock()
	now := time.Now()
	if !h.lastLogged.IsZero() && now.Sub(h.lastLogged) < h.interval {
		h.suppressed++
		h.mu.Unlock()
		return
	}
	suppressed := h.suppressed
	h.suppressed = 0
	h.lastLogged = now
	h.mu.Unlock()

	h.logger.Error("OpenTelemetry internal error",
		zap.Error(err),
		zap.Uint64("suppressed_errors", suppressed),
	)
}

func resolveBatchProcessorConfig(cfg TracingConfig) (batchProcessorConfig, error) {
	if cfg.BatchTimeout < 0 {
		return batchProcessorConfig{}, fmt.Errorf("goslogx: tracing batch timeout must not be negative")
	}
	if cfg.ExportTimeout < 0 {
		return batchProcessorConfig{}, fmt.Errorf("goslogx: tracing export timeout must not be negative")
	}
	if cfg.MaxQueueSize < 0 {
		return batchProcessorConfig{}, fmt.Errorf("goslogx: tracing max queue size must not be negative")
	}
	if cfg.MaxExportBatchSize < 0 {
		return batchProcessorConfig{}, fmt.Errorf("goslogx: tracing max export batch size must not be negative")
	}

	resolved := batchProcessorConfig{
		BatchTimeout:       cfg.BatchTimeout,
		ExportTimeout:      cfg.ExportTimeout,
		MaxQueueSize:       cfg.MaxQueueSize,
		MaxExportBatchSize: cfg.MaxExportBatchSize,
	}
	if resolved.BatchTimeout == 0 {
		resolved.BatchTimeout = DefaultBatchTimeout
	}
	if resolved.ExportTimeout == 0 {
		resolved.ExportTimeout = DefaultExportTimeout
	}
	if resolved.MaxQueueSize == 0 {
		resolved.MaxQueueSize = DefaultMaxQueueSize
	}
	if resolved.MaxExportBatchSize == 0 {
		resolved.MaxExportBatchSize = min(DefaultMaxExportBatchSize, resolved.MaxQueueSize)
	}
	if resolved.MaxExportBatchSize > resolved.MaxQueueSize {
		return batchProcessorConfig{}, fmt.Errorf("goslogx: tracing max export batch size must not exceed max queue size")
	}
	return resolved, nil
}

func newBatchProcessorOptions(cfg TracingConfig) ([]sdktrace.BatchSpanProcessorOption, error) {
	resolved, err := resolveBatchProcessorConfig(cfg)
	if err != nil {
		return nil, err
	}
	return []sdktrace.BatchSpanProcessorOption{
		sdktrace.WithBatchTimeout(resolved.BatchTimeout),
		sdktrace.WithExportTimeout(resolved.ExportTimeout),
		sdktrace.WithMaxQueueSize(resolved.MaxQueueSize),
		sdktrace.WithMaxExportBatchSize(resolved.MaxExportBatchSize),
	}, nil
}

func resolvedResourceConfig(cfg *Config) *Config {
	resolved := *cfg
	if cfg.Resource.ServiceVersion != "" {
		resolved.Tracing.ServiceVersion = cfg.Resource.ServiceVersion
	}
	if cfg.Resource.DeploymentEnvironment != "" {
		resolved.Tracing.DeploymentEnvironment = cfg.Resource.DeploymentEnvironment
	}
	if cfg.Resource.ServiceInstanceID != "" {
		resolved.Tracing.ServiceInstanceID = cfg.Resource.ServiceInstanceID
	}
	resolved.Tracing.ResourceAttributes = cloneStringMap(cfg.Tracing.ResourceAttributes)
	if resolved.Tracing.ResourceAttributes == nil {
		resolved.Tracing.ResourceAttributes = make(map[string]string)
	}
	for key, value := range cfg.Resource.ResourceAttributes {
		resolved.Tracing.ResourceAttributes[key] = value
	}
	return &resolved
}

func validateResourceConfig(cfg *Config) error {
	cfg = resolvedResourceConfig(cfg)
	if strings.TrimSpace(cfg.ServiceName) == "" || cfg.ServiceName == "unknown" {
		return fmt.Errorf("goslogx: service name is required when tracing is enabled")
	}
	if strings.TrimSpace(cfg.Tracing.ServiceVersion) == "" {
		return fmt.Errorf("goslogx: tracing service version is required")
	}
	if strings.TrimSpace(cfg.Tracing.DeploymentEnvironment) == "" {
		return fmt.Errorf("goslogx: tracing deployment environment is required")
	}
	if strings.TrimSpace(cfg.Tracing.ServiceInstanceID) == "" {
		return fmt.Errorf("goslogx: tracing service instance ID is required")
	}
	for key := range cfg.Tracing.ResourceAttributes {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("goslogx: tracing resource attributes contain an empty key")
		}
		if _, reserved := reservedResourceAttributeKeys[key]; reserved {
			return fmt.Errorf("goslogx: tracing resource attributes cannot override required service attributes")
		}
	}
	return nil
}

func newServiceResource(cfg *Config) (*resource.Resource, error) {
	cfg = resolvedResourceConfig(cfg)
	if err := validateResourceConfig(cfg); err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(cfg.Tracing.ResourceAttributes))
	for key := range cfg.Tracing.ResourceAttributes {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	attributes := make([]attribute.KeyValue, 0, len(keys)+4)
	for _, key := range keys {
		attributes = append(attributes, attribute.String(key, cfg.Tracing.ResourceAttributes[key]))
	}
	attributes = append(attributes,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.Tracing.ServiceVersion),
		semconv.ServiceInstanceID(cfg.Tracing.ServiceInstanceID),
		semconv.DeploymentEnvironmentName(cfg.Tracing.DeploymentEnvironment),
	)

	return resource.Merge(resource.Default(), resource.NewSchemaless(attributes...))
}

func validateTracingPort(port string) error {
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("goslogx: tracing endpoint has invalid port %q", port)
	}
	return nil
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(ctx context.Context, cfg *Config) (*sdktrace.TracerProvider, error) {
	serviceResource, err := newServiceResource(cfg)
	if err != nil {
		return nil, err
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(serviceResource),
		sdktrace.WithSampler(newTraceSampler(cfg.Tracing)),
	}
	exporter, err := newTraceExporter(ctx, cfg.Tracing)
	if err != nil {
		return nil, err
	}
	batchOptions, err := newBatchProcessorOptions(cfg.Tracing)
	if err != nil {
		return nil, err
	}
	options = append(options, sdktrace.WithBatcher(exporter, batchOptions...))

	return sdktrace.NewTracerProvider(options...), nil
}

func newTraceExporter(ctx context.Context, cfg TracingConfig) (sdktrace.SpanExporter, error) {
	return otlptracegrpc.New(ctx, newTraceExporterOptions(cfg)...)
}

func newTraceExporterOptions(cfg TracingConfig) []otlptracegrpc.Option {
	options := make([]otlptracegrpc.Option, 0, 6)
	if strings.Contains(cfg.Endpoint, "://") {
		options = append(options, otlptracegrpc.WithEndpointURL(cfg.Endpoint))
	} else {
		options = append(options, otlptracegrpc.WithEndpoint(cfg.Endpoint))
	}
	if cfg.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	} else if cfg.TLSConfig != nil {
		options = append(options, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(cloneTLSConfig(cfg.TLSConfig))))
	}
	if len(cfg.Headers) > 0 {
		options = append(options, otlptracegrpc.WithHeaders(cfg.Headers))
	}
	if cfg.Compression != "" {
		options = append(options, otlptracegrpc.WithCompressor(cfg.Compression))
	}
	if cfg.ExportTimeout > 0 {
		options = append(options, otlptracegrpc.WithTimeout(cfg.ExportTimeout))
	}
	return options
}

func cloneTLSConfig(config *tls.Config) *tls.Config {
	if config == nil {
		return nil
	}
	return config.Clone()
}

// StartTrace starts a span using the tracer configured by New and returns a
// derived context plus an end function. It exposes no OpenTelemetry types to
// the application. If tracing is disabled or failed to initialize, it returns
// the input context and a no-op end function.
func (l *Logger) StartTrace(ctx context.Context, name string) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil || l.tracer == nil || (l.tracerProvider != nil && l.tracingClosed.Load()) {
		return ctx, noopEndTrace
	}

	ctx, span := l.tracer.Start(ctx, name)
	return ctx, func() {
		span.End()
	}
}

// StartTrace starts a span using the global logger configured by New. The
// application does not need to import OpenTelemetry packages.
func StartTrace(ctx context.Context, name string) (context.Context, func()) {
	return globalLog.Load().StartTrace(ctx, name)
}

// ForceFlush exports pending telemetry from logger-owned providers.
func (l *Logger) ForceFlush(ctx context.Context) (err error) {
	if l == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	defer func() {
		l.internalMetrics.recordForceFlush(ctx, time.Since(started), err)
	}()

	if l.tracerProvider == nil && l.ownedMeterProvider == nil {
		return nil
	}
	select {
	case l.lifecycleGate <- struct{}{}:
		defer func() { <-l.lifecycleGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if l.tracingClosed.Load() {
		if l.tracerProvider != nil {
			return ErrTracingShutdown
		}
		return ErrMetricsShutdown
	}
	if l.ownedMeterProvider != nil {
		err = l.ownedMeterProvider.ForceFlush(ctx)
	}
	if l.tracerProvider != nil {
		err = errors.Join(err, l.tracerProvider.ForceFlush(ctx))
	}
	return err
}

// TracerProvider returns the provider configured for this logger. The caller
// can pass it to instrumentation such as otelhttp or otelgrpc without changing
// OpenTelemetry global state. The returned provider may be application-owned.
func (l *Logger) TracerProvider() trace.TracerProvider {
	if l == nil {
		return nil
	}
	return l.configuredProvider
}

// TracerProvider returns the provider configured on the global logger.
func TracerProvider() trace.TracerProvider {
	return globalLog.Load().TracerProvider()
}

// ForceFlush exports all ended spans queued by the global logger.
func ForceFlush(ctx context.Context) error {
	return globalLog.Load().ForceFlush(ctx)
}

// Shutdown idempotently stops owned metric and trace providers. External
// providers remain application-owned. Stop application work before calling.
func (l *Logger) Shutdown(ctx context.Context) (err error) {
	if l == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	defer func() {
		l.internalMetrics.recordShutdown(ctx, time.Since(started), err)
	}()

	if l.tracerProvider == nil && l.ownedMeterProvider == nil {
		return nil
	}
	select {
	case l.lifecycleGate <- struct{}{}:
		defer func() { <-l.lifecycleGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	l.shutdownOnce.Do(func() {
		l.tracingClosed.Store(true)
		if l.ownedMeterProvider != nil {
			l.shutdownErr = l.ownedMeterProvider.Shutdown(ctx)
		}
		if l.tracerProvider != nil {
			l.shutdownErr = errors.Join(l.shutdownErr, l.tracerProvider.Shutdown(ctx))
		}
		telemetryGlobalsMu.Lock()
		defer telemetryGlobalsMu.Unlock()
		if l.registeredGlobalMetrics && otel.GetMeterProvider() == l.ownedMeterProvider {
			otel.SetMeterProvider(l.previousMeterProvider)
		}
		if l.registeredGlobal && otel.GetTracerProvider() == l.tracerProvider {
			otel.SetTracerProvider(l.previousProvider)
		}
		if l.registeredGlobal && otel.GetTextMapPropagator() == l.installedPropagator {
			otel.SetTextMapPropagator(l.previousPropagator)
		}
		if current, ok := otel.GetErrorHandler().(*rateLimitedOTelErrorHandler); ok && current == l.otelHandler {
			otel.SetErrorHandler(l.previousOTelHandler)
		}
	})
	return l.shutdownErr
}

// Shutdown flushes pending spans and stops the global logger's TracerProvider.
func Shutdown(ctx context.Context) error {
	return globalLog.Load().Shutdown(ctx)
}

// TracingError returns an error encountered while initializing tracing. Logging
// remains available when tracing initialization fails.
func (l *Logger) TracingError() error {
	if l == nil {
		return nil
	}
	return l.tracingErr
}

// TracingError returns an error encountered while initializing tracing on the
// global logger.
func TracingError() error {
	return globalLog.Load().TracingError()
}

// ExtractHTTPContext extracts W3C Trace Context and baggage from HTTP headers
// using the propagator configured by goslogx.
func ExtractHTTPContext(ctx context.Context, header http.Header) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return newPropagator().Extract(ctx, propagation.HeaderCarrier(header))
}

// InjectHTTPContext injects W3C Trace Context and baggage into HTTP headers.
func InjectHTTPContext(ctx context.Context, header http.Header) {
	if ctx == nil {
		ctx = context.Background()
	}
	newPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}

// TextMapPropagator returns the W3C Trace Context and Baggage propagator used
// by goslogx helpers. It can be supplied directly to boundary instrumentation.
func TextMapPropagator() propagation.TextMapPropagator {
	return newPropagator()
}

func recordSpanError(ctx context.Context, err error) {
	if ctx == nil || err == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
