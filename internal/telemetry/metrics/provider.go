package metrics

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	tracing "github.com/muhammadluth/goslogx/internal/telemetry/tracing"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// DefaultMetricExportInterval controls the default asynchronous export cadence.
const DefaultMetricExportInterval = time.Minute

// MetricsConfig controls logger-owned OTLP/gRPC metrics.
// Service resource identity comes from WithResource (or legacy tracing fields).
// Zero export interval/timeout use DefaultMetricExportInterval/DefaultExportTimeout.
// RegisterGlobal is opt-in; TLS and header validation follow TracingConfig.
type Config struct {
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

func Validate(cfg Config) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.ExportInterval < 0 {
		return fmt.Errorf("goslogx: metrics export interval must not be negative")
	}
	err := tracing.Validate(tracing.Config{
		Enabled: true, Endpoint: cfg.Endpoint, Insecure: cfg.Insecure,
		TLSConfig: cfg.TLSConfig, Headers: cfg.Headers, Compression: cfg.Compression,
		ExportTimeout: cfg.ExportTimeout, ErrorLogInterval: cfg.ErrorLogInterval,
	})
	if err != nil {
		return errors.New(strings.ReplaceAll(err.Error(), "tracing", "metrics"))
	}
	return nil
}
func Setup(config Config, external metric.MeterProvider, serviceResource *resource.Resource, resourceErr error) (*sdkmetric.MeterProvider, error) {
	if !config.Enabled {
		return nil, nil
	}
	if external != nil {
		return nil, fmt.Errorf("goslogx: WithMetrics and WithMeterProvider cannot be combined")
	}
	if err := Validate(config); err != nil {
		return nil, err
	}
	if resourceErr != nil {
		return nil, errors.New(strings.ReplaceAll(resourceErr.Error(), "tracing", "metrics"))
	}
	exporter, err := NewExporter(context.Background(), config)
	if err != nil {
		return nil, err
	}
	timeout := ExportTimeout(config)
	interval := config.ExportInterval
	if interval == 0 {
		interval = DefaultMetricExportInterval
	}
	reader := NewReader(exporter, interval, timeout)
	return sdkmetric.NewMeterProvider(sdkmetric.WithResource(serviceResource), sdkmetric.WithReader(reader)), nil
}
