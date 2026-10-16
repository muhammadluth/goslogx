package propagationimpl

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
)

func Legacy() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}
func Default() propagation.TextMapPropagator { return propagation.TraceContext{} }
func Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return Legacy().Extract(ctx, carrier)
}
func Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	if ctx == nil {
		ctx = context.Background()
	}
	Legacy().Inject(ctx, carrier)
}
