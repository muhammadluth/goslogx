package goslogx

import (
	"context"
	"crypto/tls"
	"time"

	lifecycleimpl "github.com/muhammadluth/goslogx/internal/telemetry/lifecycle"
	metricsimpl "github.com/muhammadluth/goslogx/internal/telemetry/metrics"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap/zapcore"
)

const DefaultMetricExportInterval = metricsimpl.DefaultMetricExportInterval

var ErrMetricsShutdown = lifecycleimpl.ErrMetricsShutdown

// MetricsError returns the managed metrics initialization error, if any.
func (l *Logger) MetricsError() error {
	if l == nil || l.telemetry == nil {
		return nil
	}
	return l.telemetry.MetricsErr
}
func MetricsError() error { return globalLog.Load().MetricsError() }

// MeterProvider returns the managed or application-owned provider configured
// for this logger. With no provider configured, it returns an explicit no-op.
func (l *Logger) MeterProvider() metric.MeterProvider {
	if l == nil || l.telemetry == nil || l.telemetry.MeterProvider == nil {
		return noop.NewMeterProvider()
	}
	return l.telemetry.MeterProvider
}

// MeterProvider returns the provider configured on the global logger. It never
// reads or changes the OpenTelemetry global MeterProvider.
func MeterProvider() metric.MeterProvider {
	return globalLog.Load().MeterProvider()
}
func (l *Logger) Meter(name string, options ...metric.MeterOption) metric.Meter {
	if name == "" {
		name = instrumentationName
	}
	return l.MeterProvider().Meter(name, options...)
}
func Meter(name string, options ...metric.MeterOption) metric.Meter {
	return globalLog.Load().Meter(name, options...)
}
func (l *Logger) recordLogMetric(ctx context.Context, severity string, level zapcore.Level) {
	if l == nil || l.telemetry == nil || l.telemetry.InternalMetrics == nil || !l.logger.Core().Enabled(level) {
		return
	}
	l.telemetry.InternalMetrics.RecordLog(ctx, severity)
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
