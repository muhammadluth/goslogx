package metrics

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Severity level constants for Cloud Logging compatibility.
// Using constants avoids string allocations on every log call.
const (
	severityDebug    = "DEBUG"
	severityInfo     = "INFO"
	severityWarning  = "WARNING"
	severityError    = "ERROR"
	severityCritical = "CRITICAL"
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

func handleInternalMetricInitializationError(name string, err error) *Recorder {
	otel.Handle(fmt.Errorf("goslogx: initialize internal metric %q: %w", name, err))
	return nil
}

type Recorder struct {
	logRecords         metric.Int64Counter
	telemetryErrors    metric.Int64Counter
	forceFlushDuration metric.Float64Histogram
	forceFlushErrors   metric.Int64Counter
	shutdownDuration   metric.Float64Histogram
	shutdownErrors     metric.Int64Counter
}

func (m *Recorder) RecordForceFlush(ctx context.Context, duration time.Duration, err error) {
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
func (m *Recorder) RecordLog(ctx context.Context, severity string) {
	if m == nil {
		return
	}

	m.logRecords.Add(metricContext(ctx), 1, severityMetricOptions(severity)...)
}

func (m *Recorder) RecordShutdown(ctx context.Context, duration time.Duration, err error) {
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
func (m *Recorder) RecordTelemetryError(ctx context.Context) {
	if m == nil {
		return
	}
	m.telemetryErrors.Add(
		metricContext(ctx),
		1,
		telemetryErrorMetricOptions...,
	)
}
func metricContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

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

func NewRecorder(provider metric.MeterProvider) *Recorder {
	if provider == nil {
		return nil
	}

	meter := provider.Meter("github.com/muhammadluth/goslogx")
	metrics := &Recorder{}
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
