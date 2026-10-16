package goslogx

import (
	"context"
	"net/http"

	propagationimpl "github.com/muhammadluth/goslogx/internal/telemetry/propagation"
	"go.opentelemetry.io/otel/propagation"
)

// TextMapPropagator retains legacy W3C Trace Context and baggage propagation.
func TextMapPropagator() propagation.TextMapPropagator { return propagationimpl.Legacy() }

// TextMapPropagator supplies privacy-safe W3C Trace Context to integrations.
func (l *Logger) TextMapPropagator() propagation.TextMapPropagator { return propagationimpl.Default() }
func ExtractHTTPContext(ctx context.Context, header http.Header) context.Context {
	return propagationimpl.Extract(ctx, propagation.HeaderCarrier(header))
}
func InjectHTTPContext(ctx context.Context, header http.Header) {
	propagationimpl.Inject(ctx, propagation.HeaderCarrier(header))
}
