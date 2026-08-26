package goslogx

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap/zapcore"
)

const (
	metricLogRecords         = "goslogx.log.records"
	metricTelemetryErrors    = "goslogx.telemetry.errors"
	metricForceFlushDuration = "goslogx.telemetry.force_flush.duration"
	metricForceFlushErrors   = "goslogx.telemetry.force_flush.errors"
	metricShutdownDuration   = "goslogx.telemetry.shutdown.duration"
	metricShutdownErrors     = "goslogx.telemetry.shutdown.errors"

	operationErrorHandler = "error_handler"
	operationForceFlush   = "force_flush"
	operationShutdown     = "shutdown"
	resultSuccess         = "success"
	resultError           = "error"
)

var (
	debugLogAttributes    = attribute.NewSet(attribute.String("severity", severityDebug))
	infoLogAttributes     = attribute.NewSet(attribute.String("severity", severityInfo))
	warningLogAttributes  = attribute.NewSet(attribute.String("severity", severityWarning))
	errorLogAttributes    = attribute.NewSet(attribute.String("severity", severityError))
	criticalLogAttributes = attribute.NewSet(attribute.String("severity", severityCritical))

	telemetryErrorAttributes = attribute.NewSet(
		attribute.String("operation", operationErrorHandler),
		attribute.String("result", resultError),
	)
	forceFlushSuccessAttributes = attribute.NewSet(
		attribute.String("operation", operationForceFlush),
		attribute.String("result", resultSuccess),
	)
	forceFlushErrorAttributes = attribute.NewSet(
		attribute.String("operation", operationForceFlush),
		attribute.String("result", resultError),
	)
	shutdownSuccessAttributes = attribute.NewSet(
		attribute.String("operation", operationShutdown),
		attribute.String("result", resultSuccess),
	)
	shutdownErrorAttributes = attribute.NewSet(
		attribute.String("operation", operationShutdown),
		attribute.String("result", resultError),
	)

	debugLogMetricOptions    = []metric.AddOption{metric.WithAttributeSet(debugLogAttributes)}
	infoLogMetricOptions     = []metric.AddOption{metric.WithAttributeSet(infoLogAttributes)}
	warningLogMetricOptions  = []metric.AddOption{metric.WithAttributeSet(warningLogAttributes)}
	errorLogMetricOptions    = []metric.AddOption{metric.WithAttributeSet(errorLogAttributes)}
	criticalLogMetricOptions = []metric.AddOption{metric.WithAttributeSet(criticalLogAttributes)}

	telemetryErrorMetricOptions = []metric.AddOption{metric.WithAttributeSet(telemetryErrorAttributes)}
	forceFlushErrorAddOptions   = []metric.AddOption{metric.WithAttributeSet(forceFlushErrorAttributes)}
	shutdownErrorAddOptions     = []metric.AddOption{metric.WithAttributeSet(shutdownErrorAttributes)}

	forceFlushSuccessRecordOptions = []metric.RecordOption{metric.WithAttributeSet(forceFlushSuccessAttributes)}
	forceFlushErrorRecordOptions   = []metric.RecordOption{metric.WithAttributeSet(forceFlushErrorAttributes)}
	shutdownSuccessRecordOptions   = []metric.RecordOption{metric.WithAttributeSet(shutdownSuccessAttributes)}
	shutdownErrorRecordOptions     = []metric.RecordOption{metric.WithAttributeSet(shutdownErrorAttributes)}
)

type internalMetrics struct {
	logRecords         metric.Int64Counter
	telemetryErrors    metric.Int64Counter
	forceFlushDuration metric.Float64Histogram
	forceFlushErrors   metric.Int64Counter
	shutdownDuration   metric.Float64Histogram
	shutdownErrors     metric.Int64Counter
}

func newInternalMetrics(provider metric.MeterProvider) *internalMetrics {
	if provider == nil {
		return nil
	}

	meter := provider.Meter(instrumentationName)
	metrics := &internalMetrics{}
	var err error

	metrics.logRecords, err = meter.Int64Counter(
		metricLogRecords,
		metric.WithDescription("Number of log records emitted by goslogx."),
	)
	if err != nil {
		return handleInternalMetricInitializationError(metricLogRecords, err)
	}
	metrics.telemetryErrors, err = meter.Int64Counter(
		metricTelemetryErrors,
		metric.WithDescription("Number of OpenTelemetry internal errors observed by goslogx."),
	)
	if err != nil {
		return handleInternalMetricInitializationError(metricTelemetryErrors, err)
	}
	metrics.forceFlushDuration, err = meter.Float64Histogram(
		metricForceFlushDuration,
		metric.WithDescription("Duration of goslogx tracing ForceFlush calls."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return handleInternalMetricInitializationError(metricForceFlushDuration, err)
	}
	metrics.forceFlushErrors, err = meter.Int64Counter(
		metricForceFlushErrors,
		metric.WithDescription("Number of failed goslogx tracing ForceFlush calls."),
	)
	if err != nil {
		return handleInternalMetricInitializationError(metricForceFlushErrors, err)
	}
	metrics.shutdownDuration, err = meter.Float64Histogram(
		metricShutdownDuration,
		metric.WithDescription("Duration of goslogx tracing Shutdown calls."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return handleInternalMetricInitializationError(metricShutdownDuration, err)
	}
	metrics.shutdownErrors, err = meter.Int64Counter(
		metricShutdownErrors,
		metric.WithDescription("Number of failed goslogx tracing Shutdown calls."),
	)
	if err != nil {
		return handleInternalMetricInitializationError(metricShutdownErrors, err)
	}

	return metrics
}

func handleInternalMetricInitializationError(name string, err error) *internalMetrics {
	otel.Handle(fmt.Errorf("goslogx: initialize internal metric %q: %w", name, err))
	return nil
}

func (l *Logger) recordLogMetric(ctx context.Context, severity string, level zapcore.Level) {
	if l == nil || l.internalMetrics == nil || !l.logger.Core().Enabled(level) {
		return
	}
	l.internalMetrics.recordLog(ctx, severity)
}

func (m *internalMetrics) recordLog(ctx context.Context, severity string) {
	if m == nil {
		return
	}

	m.logRecords.Add(metricContext(ctx), 1, severityMetricOptions(severity)...)
}

func (m *internalMetrics) recordTelemetryError(ctx context.Context) {
	if m == nil {
		return
	}
	m.telemetryErrors.Add(
		metricContext(ctx),
		1,
		telemetryErrorMetricOptions...,
	)
}

func (m *internalMetrics) recordForceFlush(ctx context.Context, duration time.Duration, err error) {
	if m == nil {
		return
	}
	recordOptions := forceFlushSuccessRecordOptions
	if err != nil {
		recordOptions = forceFlushErrorRecordOptions
		m.forceFlushErrors.Add(metricContext(ctx), 1, forceFlushErrorAddOptions...)
	}
	m.forceFlushDuration.Record(
		metricContext(ctx),
		duration.Seconds(),
		recordOptions...,
	)
}

func (m *internalMetrics) recordShutdown(ctx context.Context, duration time.Duration, err error) {
	if m == nil {
		return
	}
	recordOptions := shutdownSuccessRecordOptions
	if err != nil {
		recordOptions = shutdownErrorRecordOptions
		m.shutdownErrors.Add(metricContext(ctx), 1, shutdownErrorAddOptions...)
	}
	m.shutdownDuration.Record(
		metricContext(ctx),
		duration.Seconds(),
		recordOptions...,
	)
}

func metricContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func severityMetricOptions(severity string) []metric.AddOption {
	switch severity {
	case severityDebug:
		return debugLogMetricOptions
	case severityWarning:
		return warningLogMetricOptions
	case severityError:
		return errorLogMetricOptions
	case severityCritical:
		return criticalLogMetricOptions
	default:
		return infoLogMetricOptions
	}
}
