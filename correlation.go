package goslogx

import (
	"context"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
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
		zap.String("obs_trace_id", spanContext.TraceID().String()),
		zap.String("obs_span_id", spanContext.SpanID().String()),
		zap.String("obs_trace_flags", spanContext.TraceFlags().String()),
	)
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
