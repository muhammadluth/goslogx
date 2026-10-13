package goslogx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/grpc/credentials"
)

// DefaultMetricExportInterval controls the default asynchronous export cadence.
const DefaultMetricExportInterval = time.Minute

// ErrMetricsShutdown indicates that managed metrics have stopped.
var ErrMetricsShutdown = errors.New("goslogx: metrics provider is shut down")

func validateMetricsConfig(cfg MetricsConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.ExportInterval < 0 {
		return fmt.Errorf("goslogx: metrics export interval must not be negative")
	}
	err := validateTracingConfig(TracingConfig{
		Enabled: true, Endpoint: cfg.Endpoint, Insecure: cfg.Insecure,
		TLSConfig: cfg.TLSConfig, Headers: cfg.Headers, Compression: cfg.Compression,
		ExportTimeout: cfg.ExportTimeout, ErrorLogInterval: cfg.ErrorLogInterval,
	})
	if err != nil {
		return errors.New(strings.ReplaceAll(err.Error(), "tracing", "metrics"))
	}
	return nil
}

func setupMetrics(cfg *Config) (*sdkmetric.MeterProvider, error) {
	if !cfg.Metrics.Enabled {
		return nil, nil
	}
	if cfg.MeterProvider != nil {
		return nil, fmt.Errorf("goslogx: WithMetrics and WithMeterProvider cannot be combined")
	}
	if err := validateMetricsConfig(cfg.Metrics); err != nil {
		return nil, err
	}
	serviceResource, err := newServiceResource(cfg)
	if err != nil {
		return nil, errors.New(strings.ReplaceAll(err.Error(), "tracing", "metrics"))
	}
	config := cfg.Metrics
	options := []otlpmetricgrpc.Option{}
	if strings.Contains(config.Endpoint, "://") {
		options = append(options, otlpmetricgrpc.WithEndpointURL(config.Endpoint))
	} else {
		options = append(options, otlpmetricgrpc.WithEndpoint(config.Endpoint))
	}
	if config.Insecure {
		options = append(options, otlpmetricgrpc.WithInsecure())
	} else if config.TLSConfig != nil {
		options = append(options, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(cloneTLSConfig(config.TLSConfig))))
	}
	if len(config.Headers) > 0 {
		options = append(options, otlpmetricgrpc.WithHeaders(config.Headers))
	}
	if config.Compression != "" {
		options = append(options, otlpmetricgrpc.WithCompressor(config.Compression))
	}
	timeout := config.ExportTimeout
	if timeout == 0 {
		timeout = DefaultExportTimeout
	}
	options = append(options, otlpmetricgrpc.WithTimeout(timeout))
	exporter, err := otlpmetricgrpc.New(context.Background(), options...)
	if err != nil {
		return nil, err
	}
	interval := config.ExportInterval
	if interval == 0 {
		interval = DefaultMetricExportInterval
	}
	reader := sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(interval), sdkmetric.WithTimeout(timeout))
	return sdkmetric.NewMeterProvider(sdkmetric.WithResource(serviceResource), sdkmetric.WithReader(reader)), nil
}

// MetricsError returns the managed metrics initialization error, if any.
func (l *Logger) MetricsError() error {
	if l == nil {
		return nil
	}
	return l.metricsErr
}

func MetricsError() error { return globalLog.Load().MetricsError() }
