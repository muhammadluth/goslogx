package goslogx

import (
	"bytes"
	"crypto/tls"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
)

func TestOptions(t *testing.T) {
	t.Run("WithServiceName", func(t *testing.T) {
		cfg := defaultConfig()
		WithServiceName("my-service")(cfg)
		if cfg.ServiceName != "my-service" {
			t.Errorf("Expected ServiceName my-service, got %s", cfg.ServiceName)
		}
	})

	t.Run("WithOutput", func(t *testing.T) {
		cfg := defaultConfig()
		buf := &bytes.Buffer{}
		WithOutput(buf)(cfg)
		if cfg.Output != buf {
			t.Errorf("Expected Output to be the buffer, got %v", cfg.Output)
		}
	})

	t.Run("WithDebug", func(t *testing.T) {
		t.Run("True", func(t *testing.T) {
			cfg := defaultConfig()
			WithDebug(true)(cfg)
			if cfg.Level != zapcore.DebugLevel {
				t.Errorf("Expected DebugLevel for WithDebug(true), got %v", cfg.Level)
			}
		})
		t.Run("False", func(t *testing.T) {
			cfg := defaultConfig()
			WithDebug(false)(cfg)
			if cfg.Level != zapcore.InfoLevel {
				t.Errorf("Expected InfoLevel for WithDebug(false), got %v", cfg.Level)
			}
		})
	})

	t.Run("WithMasking", func(t *testing.T) {
		t.Run("True", func(t *testing.T) {
			cfg := defaultConfig()
			WithMasking(true)(cfg)
			if !cfg.Masking.Enabled {
				t.Error("Expected Masking.Enabled to be true")
			}
		})
		t.Run("False", func(t *testing.T) {
			cfg := defaultConfig()
			WithMasking(false)(cfg)
			if cfg.Masking.Enabled {
				t.Error("Expected Masking.Enabled to be false")
			}
		})
	})

	t.Run("WithTracing", func(t *testing.T) {
		cfg := defaultConfig()
		WithTracing(TracingConfig{Endpoint: "otel-collector:4317", SamplingRatio: 0.1})(cfg)
		if !cfg.Tracing.Enabled {
			t.Errorf("unexpected tracing config: %#v", cfg.Tracing)
		}
		if cfg.Tracing.Endpoint != "otel-collector:4317" {
			t.Errorf("Endpoint = %q", cfg.Tracing.Endpoint)
		}
		if cfg.Tracing.SamplingRatio != 0.1 {
			t.Errorf("SamplingRatio = %v", cfg.Tracing.SamplingRatio)
		}
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.ServiceName != "unknown" {
		t.Errorf("Expected default ServiceName unknown, got %s", cfg.ServiceName)
	}
	if cfg.Level != zapcore.InfoLevel {
		t.Errorf("Expected default Level InfoLevel, got %v", cfg.Level)
	}
	if cfg.Output != os.Stdout {
		t.Errorf("Expected default Output os.Stdout, got %v", cfg.Output)
	}
	if !cfg.Masking.Enabled {
		t.Error("Expected default Masking.Enabled to be true")
	}
}

func TestTracingConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  TracingConfig
		wantErr bool
	}{
		{
			name:   "disabled without endpoint",
			config: TracingConfig{},
		},
		{
			name:   "host and port",
			config: TracingConfig{Enabled: true, Endpoint: "otel-collector:4317"},
		},
		{
			name:   "HTTP URL",
			config: TracingConfig{Enabled: true, Endpoint: "http://otel-collector:4317", Insecure: true},
		},
		{
			name:   "HTTPS URL",
			config: TracingConfig{Enabled: true, Endpoint: "https://telemetry.example.com"},
		},
		{
			name:    "missing endpoint",
			config:  TracingConfig{Enabled: true},
			wantErr: true,
		},
		{
			name:    "missing port",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector"},
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			config:  TracingConfig{Enabled: true, Endpoint: "grpc://otel-collector:4317"},
			wantErr: true,
		},
		{
			name:    "invalid port",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector:70000"},
			wantErr: true,
		},
		{
			name:    "unexpected path",
			config:  TracingConfig{Enabled: true, Endpoint: "https://telemetry.example.com/v1/traces"},
			wantErr: true,
		},
		{
			name:    "surrounding whitespace",
			config:  TracingConfig{Enabled: true, Endpoint: " otel-collector:4317"},
			wantErr: true,
		},
		{
			name:    "sampling ratio below zero",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector:4317", SamplingRatio: -0.1},
			wantErr: true,
		},
		{
			name:    "sampling ratio above one",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector:4317", SamplingRatio: 1.1},
			wantErr: true,
		},
		{
			name:    "HTTP without insecure",
			config:  TracingConfig{Enabled: true, Endpoint: "http://otel-collector:4317"},
			wantErr: true,
		},
		{
			name:    "HTTPS with insecure",
			config:  TracingConfig{Enabled: true, Endpoint: "https://otel-collector:4317", Insecure: true},
			wantErr: true,
		},
		{
			name:    "TLS with insecure",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector:4317", Insecure: true, TLSConfig: &tls.Config{}},
			wantErr: true,
		},
		{
			name:    "unsupported compression",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector:4317", Compression: "zstd"},
			wantErr: true,
		},
		{
			name:    "negative export timeout",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector:4317", ExportTimeout: -time.Second},
			wantErr: true,
		},
		{
			name:    "negative batch timeout",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector:4317", BatchTimeout: -time.Second},
			wantErr: true,
		},
		{
			name:    "negative queue size",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector:4317", MaxQueueSize: -1},
			wantErr: true,
		},
		{
			name:    "negative batch size",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector:4317", MaxExportBatchSize: -1},
			wantErr: true,
		},
		{
			name:    "batch exceeds queue",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector:4317", MaxQueueSize: 100, MaxExportBatchSize: 101},
			wantErr: true,
		},
		{
			name:    "negative error log interval",
			config:  TracingConfig{Enabled: true, Endpoint: "otel-collector:4317", ErrorLogInterval: -time.Second},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTracingConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTracingConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBatchProcessorConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		config, err := resolveBatchProcessorConfig(TracingConfig{})
		if err != nil {
			t.Fatalf("resolveBatchProcessorConfig() error = %v", err)
		}
		if config.BatchTimeout != DefaultBatchTimeout ||
			config.ExportTimeout != DefaultExportTimeout ||
			config.MaxQueueSize != DefaultMaxQueueSize ||
			config.MaxExportBatchSize != DefaultMaxExportBatchSize {
			t.Errorf("unexpected defaults: %#v", config)
		}
	})

	t.Run("custom", func(t *testing.T) {
		input := TracingConfig{
			BatchTimeout:       2 * time.Second,
			ExportTimeout:      3 * time.Second,
			MaxQueueSize:       100,
			MaxExportBatchSize: 25,
		}
		config, err := resolveBatchProcessorConfig(input)
		if err != nil {
			t.Fatalf("resolveBatchProcessorConfig() error = %v", err)
		}
		if config.BatchTimeout != input.BatchTimeout ||
			config.ExportTimeout != input.ExportTimeout ||
			config.MaxQueueSize != input.MaxQueueSize ||
			config.MaxExportBatchSize != input.MaxExportBatchSize {
			t.Errorf("resolved config = %#v, want %#v", config, input)
		}
		options, err := newBatchProcessorOptions(input)
		if err != nil {
			t.Fatalf("newBatchProcessorOptions() error = %v", err)
		}
		if len(options) != 4 {
			t.Errorf("batch options = %d, want 4", len(options))
		}
	})

	t.Run("default batch is capped by custom queue", func(t *testing.T) {
		config, err := resolveBatchProcessorConfig(TracingConfig{MaxQueueSize: 100})
		if err != nil {
			t.Fatalf("resolveBatchProcessorConfig() error = %v", err)
		}
		if config.MaxExportBatchSize != 100 {
			t.Errorf("MaxExportBatchSize = %d, want 100", config.MaxExportBatchSize)
		}
	})
}

func TestTracingTransportConfig(t *testing.T) {
	tlsConfig := &tls.Config{ServerName: "telemetry.example.com", MinVersion: tls.VersionTLS12}
	headers := map[string]string{"authorization": "Bearer secret"}
	resourceAttributes := map[string]string{"team.name": "payments"}
	config := defaultConfig()
	WithTracing(TracingConfig{
		Endpoint:           "telemetry.example.com:4317",
		TLSConfig:          tlsConfig,
		Headers:            headers,
		Compression:        "gzip",
		ExportTimeout:      5 * time.Second,
		ResourceAttributes: resourceAttributes,
	})(config)

	if config.Tracing.TLSConfig == tlsConfig {
		t.Error("TLSConfig was not cloned")
	}
	if config.Tracing.Headers["authorization"] != headers["authorization"] {
		t.Error("headers were not copied")
	}
	if config.Tracing.ResourceAttributes["team.name"] != resourceAttributes["team.name"] {
		t.Error("resource attributes were not copied")
	}
	if options := newTraceExporterOptions(config.Tracing); len(options) != 5 {
		t.Errorf("exporter options = %d, want 5", len(options))
	}

	tlsConfig.ServerName = "changed.example.com"
	headers["authorization"] = "changed"
	resourceAttributes["team.name"] = "changed"
	if config.Tracing.TLSConfig.ServerName != "telemetry.example.com" {
		t.Error("stored TLSConfig changed after caller mutation")
	}
	if config.Tracing.Headers["authorization"] != "Bearer secret" {
		t.Error("stored headers changed after caller mutation")
	}
	if config.Tracing.ResourceAttributes["team.name"] != "payments" {
		t.Error("stored resource attributes changed after caller mutation")
	}
}

func TestTracingValidationDoesNotLeakHeaderValues(t *testing.T) {
	const secret = "Bearer production-secret"
	err := validateTracingConfig(TracingConfig{
		Enabled:  true,
		Endpoint: "telemetry.example.com:4317",
		Headers:  map[string]string{"authorization": secret + "\n"},
	})
	if err == nil {
		t.Fatal("validateTracingConfig() error = nil, want invalid header error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("validation error leaked header value: %v", err)
	}
}

func TestTracingValidationDoesNotLeakEndpointCredentials(t *testing.T) {
	const secret = "production-secret"
	err := validateTracingConfig(TracingConfig{
		Enabled:  true,
		Endpoint: "https://collector-user:" + secret + "@telemetry.example.com:4317",
	})
	if err == nil {
		t.Fatal("validateTracingConfig() error = nil, want invalid endpoint error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("validation error leaked endpoint credentials: %v", err)
	}
}

func TestTracingSecurityValidation(t *testing.T) {
	tests := []struct {
		name   string
		config TracingConfig
	}{
		{
			name: "TLS below 1.2",
			config: TracingConfig{
				TLSConfig: &tls.Config{MinVersion: tls.VersionTLS11},
			},
		},
		{
			name: "uppercase metadata key",
			config: TracingConfig{
				Headers: map[string]string{"Authorization": "Bearer token"},
			},
		},
		{
			name: "reserved gRPC metadata key",
			config: TracingConfig{
				Headers: map[string]string{"grpc-timeout": "1S"},
			},
		},
		{
			name: "control character in metadata value",
			config: TracingConfig{
				Headers: map[string]string{"authorization": "Bearer token\x00"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.config.Enabled = true
			test.config.Endpoint = "telemetry.example.com:4317"
			if err := validateTracingConfig(test.config); err == nil {
				t.Fatal("validateTracingConfig() error = nil, want security validation error")
			}
		})
	}
}
