package tracing

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ScopeName = "github.com/muhammadluth/goslogx"

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

// TracingConfig controls OpenTelemetry tracing initialization.
type Config struct {
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

func Validate(cfg Config) error {
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
	if _, err := ResolveBatchConfig(cfg); err != nil {
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
			return validatePort(port)
		}
		return nil
	}

	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || host == "" {
		return fmt.Errorf("goslogx: tracing endpoint must use host:port or http(s)://host:port")
	}
	return validatePort(port)
}
func validatePort(port string) error {
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("goslogx: tracing endpoint has invalid port %q", port)
	}
	return nil
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
