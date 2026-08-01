package goslogx

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// appendOTELFields appends trace correlation fields from a valid SpanContext.
// It deliberately reads only the SpanContext and never records or exports data.
func appendOTELFields(ctx context.Context, fields []zap.Field) []zap.Field {
	if ctx == nil {
		return fields
	}

	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return fields
	}

	return append(fields,
		zap.String("trace_id", spanContext.TraceID().String()),
		zap.String("span_id", spanContext.SpanID().String()),
		zap.String("trace_flags", spanContext.TraceFlags().String()),
	)
}

// DebugContext logs a debug message and adds trace correlation fields from ctx
// when it contains a valid OpenTelemetry SpanContext. When library-managed
// tracing is enabled and ctx has no span, it creates a short span automatically.
func (l *Logger) DebugContext(ctx context.Context, module string, msgType MsgType, msg string, data any) {
	if !l.logger.Core().Enabled(zapcore.DebugLevel) {
		return
	}
	ctx, endTrace := l.ensureTraceContext(ctx, "log.debug."+module)
	defer endTrace()

	fields := getFields()
	defer putFields(fields)

	fields = appendOTELFields(ctx, fields)
	fields = append(fields,
		zap.String("module", module),
		zap.String("msg_type", string(msgType)),
		zap.String("severity", severityDebug),
	)
	if data != nil {
		fields = append(fields, dataField("data", data))
	}

	callerSkip := detectCallerSkip()
	logger := l.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(callerSkip))
	logger.Log(zapcore.DebugLevel, msg, fields...)
}

// DebugContext logs through the global logger and adds trace correlation fields
// using the tracing and output configuration established by New.
func DebugContext(ctx context.Context, module string, msgType MsgType, msg string, data any) {
	globalLog.Load().DebugContext(ctx, module, msgType, msg, data)
}

// InfoContext logs an informational message and adds trace correlation fields
// from ctx when it contains a valid OpenTelemetry SpanContext. When
// library-managed tracing is enabled and ctx has no span, it creates a short
// span automatically.
func (l *Logger) InfoContext(ctx context.Context, module string, msgType MsgType, msg string, data any) {
	ctx, endTrace := l.ensureTraceContext(ctx, "log.info."+module)
	defer endTrace()

	fields := getFields()
	defer putFields(fields)

	fields = appendOTELFields(ctx, fields)
	fields = append(fields,
		zap.String("module", module),
		zap.String("msg_type", string(msgType)),
		zap.String("severity", severityInfo),
	)
	if data != nil {
		fields = append(fields, dataField("data", data))
	}

	callerSkip := detectCallerSkip()
	logger := l.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(callerSkip))
	logger.Log(zapcore.InfoLevel, msg, fields...)
}

// InfoContext logs through the global logger and adds trace correlation fields
// using the tracing and output configuration established by New.
func InfoContext(ctx context.Context, module string, msgType MsgType, msg string, data any) {
	globalLog.Load().InfoContext(ctx, module, msgType, msg, data)
}

// WarningContext logs a warning message and adds trace correlation fields from
// ctx when valid. When library-managed tracing is enabled and ctx has no span,
// it creates a short span automatically.
func (l *Logger) WarningContext(ctx context.Context, module string, msg string, data any) {
	ctx, endTrace := l.ensureTraceContext(ctx, "log.warning."+module)
	defer endTrace()

	fields := getFields()
	defer putFields(fields)

	fields = appendOTELFields(ctx, fields)
	fields = append(fields,
		zap.String("module", module),
		zap.String("severity", severityWarning),
	)
	if data != nil {
		fields = append(fields, dataField("data", data))
	}

	callerSkip := detectCallerSkip()
	logger := l.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(callerSkip))
	logger.Log(zapcore.WarnLevel, msg, fields...)
}

// WarningContext logs through the global logger and adds trace correlation
// fields using the tracing and output configuration established by New.
func WarningContext(ctx context.Context, module string, msg string, data any) {
	globalLog.Load().WarningContext(ctx, module, msg, data)
}

// ErrorContext logs an error and adds trace correlation fields from ctx when
// valid. It records the error on the active span. When library-managed tracing
// is enabled and ctx has no span, it creates a short span automatically.
func (l *Logger) ErrorContext(ctx context.Context, module string, err error) {
	ctx, endTrace := l.ensureTraceContext(ctx, "log.error."+module)
	defer endTrace()

	recordSpanError(ctx, err)

	fields := getFields()
	defer putFields(fields)

	fields = appendOTELFields(ctx, fields)
	fields = append(fields,
		zap.String("module", module),
		zap.Error(err),
		zap.String("severity", severityError),
	)

	callerSkip := detectCallerSkip()
	logger := l.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(callerSkip))
	logger.Log(zapcore.ErrorLevel, "error occurred", fields...)
}

// ErrorContext logs through the global logger and adds trace correlation fields
// using the tracing and output configuration established by New.
func ErrorContext(ctx context.Context, module string, err error) {
	globalLog.Load().ErrorContext(ctx, module, err)
}

// FatalContext logs a critical error, adds trace correlation fields from ctx
// when valid, records the error on the span, and terminates the process using
// the existing fatal behavior. It creates a short span when tracing is enabled
// and ctx has no active span.
func (l *Logger) FatalContext(ctx context.Context, module string, err error) {
	ctx, endTrace := l.ensureTraceContext(ctx, "log.fatal."+module)
	recordSpanError(ctx, err)

	fields := getFields()
	defer putFields(fields)

	fields = appendOTELFields(ctx, fields)
	fields = append(fields,
		zap.String("module", module),
		zap.Error(err),
		zap.String("severity", severityCritical),
	)

	callerSkip := detectCallerSkip()
	logger := l.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(callerSkip))
	endTrace()
	logger.Log(zapcore.FatalLevel, "fatal error occurred", fields...)
}

// FatalContext logs through the global logger with trace correlation from a
// valid OpenTelemetry SpanContext, then terminates the process. It uses the
// tracing and output configuration established by New.
func FatalContext(ctx context.Context, module string, err error) {
	globalLog.Load().FatalContext(ctx, module, err)
}
