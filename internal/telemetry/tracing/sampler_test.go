package tracing

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceSampler(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("parse trace ID: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("0123456789abcdef")
	if err != nil {
		t.Fatalf("parse span ID: %v", err)
	}
	t.Run("default samples root span", func(t *testing.T) {
		result := NewSampler(Config{}).ShouldSample(sdktrace.SamplingParameters{TraceID: traceID, Name: "root"})
		if result.Decision != sdktrace.RecordAndSample {
			t.Errorf("sampling decision = %v, want RecordAndSample", result.Decision)
		}
	})

	for _, tt := range []struct {
		name       string
		traceFlags trace.TraceFlags
		want       sdktrace.SamplingDecision
	}{
		{name: "sampled remote parent", traceFlags: trace.FlagsSampled, want: sdktrace.RecordAndSample},
		{name: "unsampled remote parent", traceFlags: 0, want: sdktrace.Drop},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parent := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    traceID,
				SpanID:     spanID,
				TraceFlags: tt.traceFlags,
				Remote:     true,
			})
			result := NewSampler(Config{SamplingRatio: 0.1}).ShouldSample(sdktrace.SamplingParameters{
				ParentContext: trace.ContextWithRemoteSpanContext(context.Background(), parent),
				TraceID:       traceID,
				Name:          "child",
			})
			if result.Decision != tt.want {
				t.Errorf("sampling decision = %v, want %v", result.Decision, tt.want)
			}
		})
	}
}
