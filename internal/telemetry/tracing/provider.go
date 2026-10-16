package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func Setup(cfg Config, external trace.TracerProvider, serviceResource *resource.Resource, resourceErr error) (trace.Tracer, *sdktrace.TracerProvider, error) {
	if external != nil {
		if cfg.Enabled {
			return nil, nil, fmt.Errorf("goslogx: WithTracing and WithTracerProvider cannot be combined")
		}
		return external.Tracer(ScopeName), nil, nil
	}
	if !cfg.Enabled {
		return nil, nil, nil
	}
	if err := Validate(cfg); err != nil {
		return nil, nil, err
	}
	if resourceErr != nil {
		return nil, nil, resourceErr
	}

	provider, err := NewProvider(context.Background(), cfg, serviceResource)
	if err != nil {
		return nil, nil, err
	}

	return provider.Tracer(ScopeName), provider, nil
}
func NewProvider(ctx context.Context, cfg Config, serviceResource *resource.Resource) (*sdktrace.TracerProvider, error) {
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(serviceResource),
		sdktrace.WithSampler(NewSampler(cfg)),
	}
	exporter, err := NewExporter(ctx, cfg)
	if err != nil {
		return nil, err
	}
	batchOptions, err := BatchOptions(cfg)
	if err != nil {
		return nil, err
	}
	options = append(options, sdktrace.WithBatcher(exporter, batchOptions...))

	return sdktrace.NewTracerProvider(options...), nil
}
