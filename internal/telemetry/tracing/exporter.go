package tracing

import (
	"context"
	"crypto/tls"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
)

func NewExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	return otlptracegrpc.New(ctx, ExporterOptions(cfg)...)
}
func ExporterOptions(cfg Config) []otlptracegrpc.Option {
	options := make([]otlptracegrpc.Option, 0, 6)
	if strings.Contains(cfg.Endpoint, "://") {
		options = append(options, otlptracegrpc.WithEndpointURL(cfg.Endpoint))
	} else {
		options = append(options, otlptracegrpc.WithEndpoint(cfg.Endpoint))
	}
	if cfg.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	} else if cfg.TLSConfig != nil {
		options = append(options, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(CloneTLSConfig(cfg.TLSConfig))))
	}
	if len(cfg.Headers) > 0 {
		options = append(options, otlptracegrpc.WithHeaders(cfg.Headers))
	}
	if cfg.Compression != "" {
		options = append(options, otlptracegrpc.WithCompressor(cfg.Compression))
	}
	if cfg.ExportTimeout > 0 {
		options = append(options, otlptracegrpc.WithTimeout(cfg.ExportTimeout))
	}
	return options
}
func CloneTLSConfig(config *tls.Config) *tls.Config {
	if config == nil {
		return nil
	}
	return config.Clone()
}
