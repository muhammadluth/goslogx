package goslogx

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func BenchmarkStartSpan(b *testing.B) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		b.Run(name, func(b *testing.B) {
			options := []Option{WithOutput(&discardWriter{})}
			if enabled {
				tp := sdktrace.NewTracerProvider()
				defer tp.Shutdown(context.Background())
				options = append(options, WithTracerProvider(tp))
			}
			logger := setupLog(options...)
			for b.Loop() {
				_, span := logger.StartSpan(context.Background(), "payment.process")
				span.End()
			}
		})
	}
}
func BenchmarkStartSpanAttributes(b *testing.B) {
	tp := sdktrace.NewTracerProvider()
	defer tp.Shutdown(context.Background())
	logger := setupLog(WithOutput(&discardWriter{}), WithTracerProvider(tp))
	options := []trace.SpanStartOption{trace.WithAttributes(attribute.String("payment.type", "debit"))}
	for b.Loop() {
		_, span := logger.StartSpan(context.Background(), "payment.process", options...)
		span.End()
	}
}
