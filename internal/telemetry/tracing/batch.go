package tracing

import (
	"fmt"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type BatchConfig struct {
	BatchTimeout       time.Duration
	ExportTimeout      time.Duration
	MaxQueueSize       int
	MaxExportBatchSize int
}

func ResolveBatchConfig(cfg Config) (BatchConfig, error) {
	if cfg.BatchTimeout < 0 {
		return BatchConfig{}, fmt.Errorf("goslogx: tracing batch timeout must not be negative")
	}
	if cfg.ExportTimeout < 0 {
		return BatchConfig{}, fmt.Errorf("goslogx: tracing export timeout must not be negative")
	}
	if cfg.MaxQueueSize < 0 {
		return BatchConfig{}, fmt.Errorf("goslogx: tracing max queue size must not be negative")
	}
	if cfg.MaxExportBatchSize < 0 {
		return BatchConfig{}, fmt.Errorf("goslogx: tracing max export batch size must not be negative")
	}

	resolved := BatchConfig{
		BatchTimeout:       cfg.BatchTimeout,
		ExportTimeout:      cfg.ExportTimeout,
		MaxQueueSize:       cfg.MaxQueueSize,
		MaxExportBatchSize: cfg.MaxExportBatchSize,
	}
	if resolved.BatchTimeout == 0 {
		resolved.BatchTimeout = DefaultBatchTimeout
	}
	if resolved.ExportTimeout == 0 {
		resolved.ExportTimeout = DefaultExportTimeout
	}
	if resolved.MaxQueueSize == 0 {
		resolved.MaxQueueSize = DefaultMaxQueueSize
	}
	if resolved.MaxExportBatchSize == 0 {
		resolved.MaxExportBatchSize = min(DefaultMaxExportBatchSize, resolved.MaxQueueSize)
	}
	if resolved.MaxExportBatchSize > resolved.MaxQueueSize {
		return BatchConfig{}, fmt.Errorf("goslogx: tracing max export batch size must not exceed max queue size")
	}
	return resolved, nil
}
func BatchOptions(cfg Config) ([]sdktrace.BatchSpanProcessorOption, error) {
	resolved, err := ResolveBatchConfig(cfg)
	if err != nil {
		return nil, err
	}
	return []sdktrace.BatchSpanProcessorOption{
		sdktrace.WithBatchTimeout(resolved.BatchTimeout),
		sdktrace.WithExportTimeout(resolved.ExportTimeout),
		sdktrace.WithMaxQueueSize(resolved.MaxQueueSize),
		sdktrace.WithMaxExportBatchSize(resolved.MaxExportBatchSize),
	}, nil
}
