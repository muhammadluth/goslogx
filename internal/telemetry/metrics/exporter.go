package metrics

import (
	"context"
	"strings"
	"time"

	tracing "github.com/muhammadluth/goslogx/internal/telemetry/tracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/grpc/credentials"
)

func NewExporter(ctx context.Context, config Config) (sdkmetric.Exporter, error) {
	options := []otlpmetricgrpc.Option{}
	if strings.Contains(config.Endpoint, "://") {
		options = append(options, otlpmetricgrpc.WithEndpointURL(config.Endpoint))
	} else {
		options = append(options, otlpmetricgrpc.WithEndpoint(config.Endpoint))
	}
	if config.Insecure {
		options = append(options, otlpmetricgrpc.WithInsecure())
	} else if config.TLSConfig != nil {
		options = append(options, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(tracing.CloneTLSConfig(config.TLSConfig))))
	}
	if len(config.Headers) > 0 {
		options = append(options, otlpmetricgrpc.WithHeaders(config.Headers))
	}
	if config.Compression != "" {
		options = append(options, otlpmetricgrpc.WithCompressor(config.Compression))
	}
	timeout := config.ExportTimeout
	if timeout == 0 {
		timeout = tracing.DefaultExportTimeout
	}
	options = append(options, otlpmetricgrpc.WithTimeout(timeout))

	return otlpmetricgrpc.New(ctx, options...)
}
func ExportTimeout(config Config) time.Duration {
	if config.ExportTimeout == 0 {
		return tracing.DefaultExportTimeout
	}
	return config.ExportTimeout
}
