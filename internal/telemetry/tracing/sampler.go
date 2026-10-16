package tracing

import (
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func NewSampler(cfg Config) sdktrace.Sampler {
	ratio := cfg.SamplingRatio
	if ratio == 0 {
		ratio = DefaultSamplingRatio
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}
