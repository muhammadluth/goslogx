package goslogx

import (
	"crypto/tls"
	"io"
	"os"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
)

// Config holds logger configuration options.
type Config struct {
	Resource ResourceConfig
	Metrics  MetricsConfig

	// ServiceName is the name of the service using this logger.
	// It will be included in all log entries as "application_name".
	ServiceName string

	// Level is the minimum log level that will be output.
	// Default: zapcore.InfoLevel
	Level zapcore.Level

	// Output is the writer where logs will be written.
	// Default: os.Stdout
	Output io.Writer

	Debug bool

	// Masking controls automatic field masking behavior.
	Masking MaskingConfig

	// Tracing controls OpenTelemetry tracing owned by goslogx.
	Tracing TracingConfig

	// TracerProvider is supplied and owned by the application. It is mutually
	// exclusive with managed Tracing.
	TracerProvider trace.TracerProvider

	// MeterProvider is supplied and owned by the application. goslogx never
	// registers, flushes, or shuts it down.
	MeterProvider metric.MeterProvider

	// InternalMetrics enables low-cardinality metrics about goslogx itself.
	// Metrics use the configured managed or application-owned MeterProvider.
	InternalMetrics bool
}

// MaskingConfig controls field masking behavior.
type MaskingConfig struct {
	// Enabled determines whether automatic field masking is active.
	// When true, struct fields tagged with log:"masked:*" will be masked.
	// Default: true
	Enabled bool
}

// TracingConfig controls OpenTelemetry tracing initialization.
type TracingConfig struct {
	// Enabled creates an OpenTelemetry TracerProvider inside goslogx.
	Enabled bool

	// RegisterGlobal explicitly installs the goslogx-owned provider as the
	// OpenTelemetry global provider. The previous global provider is restored
	// during Shutdown. The default is false.
	RegisterGlobal bool

	// Endpoint is the OTLP/gRPC collector URL.
	Endpoint string

	// SamplingRatio is the fraction of root traces to sample, from 0.0 to 1.0.
	// The zero value uses DefaultSamplingRatio. Parent sampling decisions are
	// always honored.
	SamplingRatio float64

	// Insecure disables transport security. Use it only for trusted local
	// Collector connections.
	Insecure bool

	// TLSConfig configures TLS, including custom root CAs and mTLS certificates.
	// It cannot be combined with Insecure.
	TLSConfig *tls.Config

	// Headers are sent with every OTLP/gRPC export request. Values commonly
	// contain credentials and are never included in validation errors.
	Headers map[string]string

	// Compression optionally enables payload compression. The supported value
	// is "gzip"; an empty value disables compression.
	Compression string

	// ExportTimeout limits each OTLP export request. The zero value uses the
	// DefaultExportTimeout. It also bounds batch processor export calls.
	ExportTimeout time.Duration

	// BatchTimeout is the maximum delay before queued spans are exported. The
	// zero value uses DefaultBatchTimeout.
	BatchTimeout time.Duration

	// MaxQueueSize is the maximum number of spans waiting for export. New spans
	// are dropped when this non-blocking queue is full. The zero value uses
	// DefaultMaxQueueSize.
	MaxQueueSize int

	// MaxExportBatchSize is the maximum number of spans per export request. The
	// zero value uses DefaultMaxExportBatchSize, capped by MaxQueueSize.
	MaxExportBatchSize int

	// ServiceVersion identifies the deployed service build.
	ServiceVersion string

	// DeploymentEnvironment identifies the deployment environment, such as
	// production or staging.
	DeploymentEnvironment string

	// ServiceInstanceID uniquely identifies this service instance.
	ServiceInstanceID string

	// ResourceAttributes adds deployment-specific resource attributes. Reserved
	// service attributes cannot be overridden through this map.
	ResourceAttributes map[string]string

	// ErrorLogInterval limits repeated OpenTelemetry internal error logs. The
	// zero value uses DefaultOTelErrorLogInterval.
	ErrorLogInterval time.Duration
}

// Option configures a Logger.
type Option func(*Config)

// ResourceConfig supplies shared identity for managed traces and metrics.
// Nonempty fields take precedence over legacy TracingConfig identity fields.
type ResourceConfig struct {
	ServiceVersion        string
	DeploymentEnvironment string
	ServiceInstanceID     string
	ResourceAttributes    map[string]string
}

// MetricsConfig controls logger-owned OTLP/gRPC metrics.
// Service resource identity comes from WithResource (or legacy tracing fields).
// Zero export interval/timeout use DefaultMetricExportInterval/DefaultExportTimeout.
// RegisterGlobal is opt-in; TLS and header validation follow TracingConfig.
type MetricsConfig struct {
	Enabled          bool
	RegisterGlobal   bool
	Endpoint         string
	Insecure         bool
	TLSConfig        *tls.Config
	Headers          map[string]string
	Compression      string
	ExportTimeout    time.Duration
	ExportInterval   time.Duration
	ErrorLogInterval time.Duration
}

// WithResource configures shared resource identity and snapshots the attribute map.
func WithResource(config ResourceConfig) Option {
	config.ResourceAttributes = cloneStringMap(config.ResourceAttributes)
	return func(c *Config) {
		copied := config
		copied.ResourceAttributes = cloneStringMap(config.ResourceAttributes)
		c.Resource = copied
	}
}

// WithMetrics enables managed metrics and snapshots transport configuration.
// It is mutually exclusive with WithMeterProvider.
func WithMetrics(config MetricsConfig) Option {
	config.Enabled = true
	config.TLSConfig = cloneTLSConfig(config.TLSConfig)
	config.Headers = cloneStringMap(config.Headers)
	return func(c *Config) {
		copied := config
		copied.TLSConfig = cloneTLSConfig(config.TLSConfig)
		copied.Headers = cloneStringMap(config.Headers)
		c.Metrics = copied
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// WithServiceName sets the service name for the logger.
// The service name appears in all log entries as "application_name".
//
// Example:
//
//	logger := goslogx.New(goslogx.WithServiceName("my-service"))
func WithServiceName(name string) Option {
	return func(c *Config) {
		c.ServiceName = name
	}
}

// WithOutput sets the output writer for logs.
// By default, logs are written to os.Stdout.
//
// Example:
//
//	file, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
//	logger := goslogx.New(
//	    goslogx.WithServiceName("my-service"),
//	    goslogx.WithOutput(file),
//	)
func WithOutput(w io.Writer) Option {
	return func(c *Config) {
		c.Output = w
	}
}

func WithDebug(debug bool) Option {
	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}
	return func(c *Config) {
		c.Level = level
	}
}

// WithMasking enables automatic field masking.
// When enabled, struct fields tagged with log:"masked:full" or log:"masked:partial"
// will be automatically masked in log output.
//
// Example:
//
//	logger := goslogx.New(
//	    goslogx.WithServiceName("my-service"),
//	    goslogx.WithMasking(true),
//	)
func WithMasking(masking bool) Option {
	return func(c *Config) {
		c.Masking.Enabled = masking
	}
}

// WithTracing enables tracing managed by goslogx using config. Applications
// only interact with goslogx APIs and do not need to import OpenTelemetry
// packages. Endpoint accepts either host:port or an http(s) URL.
func WithTracing(config TracingConfig) Option {
	return func(c *Config) {
		config.Enabled = true
		if config.TLSConfig != nil {
			config.TLSConfig = config.TLSConfig.Clone()
		}
		if config.Headers != nil {
			headers := make(map[string]string, len(config.Headers))
			for key, value := range config.Headers {
				headers[key] = value
			}
			config.Headers = headers
		}
		if config.ResourceAttributes != nil {
			attributes := make(map[string]string, len(config.ResourceAttributes))
			for key, value := range config.ResourceAttributes {
				attributes[key] = value
			}
			config.ResourceAttributes = attributes
		}
		c.Tracing = config
	}
}

// WithTracerProvider uses an application-owned OpenTelemetry TracerProvider.
// goslogx never registers or shuts down an external provider. It is mutually
// exclusive with WithTracing.
func WithTracerProvider(provider trace.TracerProvider) Option {
	return func(c *Config) {
		c.TracerProvider = provider
	}
}

// WithMeterProvider uses an application-owned OpenTelemetry MeterProvider.
// goslogx never registers, flushes, or shuts down the provider. A nil provider
// is resolved to a no-op provider when the Logger is created.
func WithMeterProvider(provider metric.MeterProvider) Option {
	return func(c *Config) {
		c.MeterProvider = provider
	}
}

// WithInternalMetrics enables or disables low-cardinality metrics describing
// log records and the internal telemetry lifecycle. The default is false.
func WithInternalMetrics(enabled bool) Option {
	return func(c *Config) {
		c.InternalMetrics = enabled
	}
}

// WithDisabledTracing disables tracing in goslogx.
func WithDisabledTracing() Option {
	return func(c *Config) {
		c.Tracing.Enabled = false
	}
}

// defaultConfig returns the default logger configuration.
func defaultConfig() *Config {
	return &Config{
		ServiceName: "unknown",
		Level:       zapcore.InfoLevel,
		Output:      os.Stdout,
		Debug:       true,
		Masking: MaskingConfig{
			Enabled: true,
		},
	}
}
