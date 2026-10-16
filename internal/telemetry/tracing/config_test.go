package tracing

import (
	"crypto/tls"
	"strings"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{{name: "disabled without endpoint", config: Config{}}, {name: "host and port", config: Config{Enabled: true, Endpoint: "otel-collector:4317"}}, {
		name:   "HTTP URL",
		config: Config{Enabled: true, Endpoint: "http://otel-collector:4317", Insecure: true},
	},
		{
			name:   "HTTPS URL",
			config: Config{Enabled: true, Endpoint: "https://telemetry.example.com"},
		},
		{
			name:    "missing endpoint",
			config:  Config{Enabled: true},
			wantErr: true,
		},
		{
			name:    "missing port",
			config:  Config{Enabled: true, Endpoint: "otel-collector"},
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			config:  Config{Enabled: true, Endpoint: "grpc://otel-collector:4317"},
			wantErr: true,
		},
		{
			name:    "invalid port",
			config:  Config{Enabled: true, Endpoint: "otel-collector:70000"},
			wantErr: true,
		},
		{
			name:    "unexpected path",
			config:  Config{Enabled: true, Endpoint: "https://telemetry.example.com/v1/traces"},
			wantErr: true,
		},
		{
			name:    "surrounding whitespace",
			config:  Config{Enabled: true, Endpoint: " otel-collector:4317"},
			wantErr: true,
		},
		{
			name:    "sampling ratio below zero",
			config:  Config{Enabled: true, Endpoint: "otel-collector:4317", SamplingRatio: -0.1},
			wantErr: true,
		},
		{
			name:    "sampling ratio above one",
			config:  Config{Enabled: true, Endpoint: "otel-collector:4317", SamplingRatio: 1.1},
			wantErr: true,
		},
		{
			name:    "HTTP without insecure",
			config:  Config{Enabled: true, Endpoint: "http://otel-collector:4317"},
			wantErr: true,
		},
		{
			name:    "HTTPS with insecure",
			config:  Config{Enabled: true, Endpoint: "https://otel-collector:4317", Insecure: true},
			wantErr: true,
		},
		{
			name:    "TLS with insecure",
			config:  Config{Enabled: true, Endpoint: "otel-collector:4317", Insecure: true, TLSConfig: &tls.Config{}},
			wantErr: true,
		},
		{
			name:    "unsupported compression",
			config:  Config{Enabled: true, Endpoint: "otel-collector:4317", Compression: "zstd"},
			wantErr: true,
		},
		{
			name:    "negative export timeout",
			config:  Config{Enabled: true, Endpoint: "otel-collector:4317", ExportTimeout: -time.Second},
			wantErr: true,
		},
		{
			name:    "negative batch timeout",
			config:  Config{Enabled: true, Endpoint: "otel-collector:4317", BatchTimeout: -time.Second},
			wantErr: true,
		},
		{
			name:    "negative queue size",
			config:  Config{Enabled: true, Endpoint: "otel-collector:4317", MaxQueueSize: -1},
			wantErr: true,
		},
		{
			name:    "negative batch size",
			config:  Config{Enabled: true, Endpoint: "otel-collector:4317", MaxExportBatchSize: -1},
			wantErr: true,
		},
		{
			name:    "batch exceeds queue",
			config:  Config{Enabled: true, Endpoint: "otel-collector:4317", MaxQueueSize: 100, MaxExportBatchSize: 101},
			wantErr: true,
		},
		{
			name:    "negative error log interval",
			config:  Config{Enabled: true, Endpoint: "otel-collector:4317", ErrorLogInterval: -time.Second},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
func TestBatchProcessorConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		config, err := ResolveBatchConfig(Config{})
		if err != nil {
			t.Fatalf("ResolveBatchConfig() error = %v", err)
		}
		if config.BatchTimeout != DefaultBatchTimeout ||
			config.ExportTimeout != DefaultExportTimeout ||
			config.MaxQueueSize != DefaultMaxQueueSize ||
			config.MaxExportBatchSize != DefaultMaxExportBatchSize {
			t.Errorf("unexpected defaults: %#v", config)
		}
	})

	t.Run("custom", func(t *testing.T) {
		input := Config{
			BatchTimeout:       2 * time.Second,
			ExportTimeout:      3 * time.Second,
			MaxQueueSize:       100,
			MaxExportBatchSize: 25,
		}
		config, err := ResolveBatchConfig(input)
		if err != nil {
			t.Fatalf("ResolveBatchConfig() error = %v", err)
		}
		if config.BatchTimeout != input.BatchTimeout ||
			config.ExportTimeout != input.ExportTimeout ||
			config.MaxQueueSize != input.MaxQueueSize ||
			config.MaxExportBatchSize != input.MaxExportBatchSize {
			t.Errorf("resolved config = %#v, want %#v", config, input)
		}
		options, err := BatchOptions(input)
		if err != nil {
			t.Fatalf("BatchOptions() error = %v", err)
		}
		if len(options) != 4 {
			t.Errorf("batch options = %d, want 4", len(options))
		}
	})

	t.Run("default batch is capped by custom queue", func(t *testing.T) {
		config, err := ResolveBatchConfig(Config{MaxQueueSize: 100})
		if err != nil {
			t.Fatalf("ResolveBatchConfig() error = %v", err)
		}
		if config.MaxExportBatchSize != 100 {
			t.Errorf("MaxExportBatchSize = %d, want 100", config.MaxExportBatchSize)
		}
	})
}
func TestTracingValidationDoesNotLeakHeaderValues(t *testing.T) {
	const secret = "Bearer production-secret"
	err := Validate(Config{
		Enabled:  true,
		Endpoint: "telemetry.example.com:4317",
		Headers:  map[string]string{"authorization": secret + "\n"},
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want invalid header error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("validation error leaked header value: %v", err)
	}
}
func TestTracingValidationDoesNotLeakEndpointCredentials(t *testing.T) {
	const secret = "production-secret"
	err := Validate(Config{
		Enabled:  true,
		Endpoint: "https://collector-user:" + secret + "@telemetry.example.com:4317",
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want invalid endpoint error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("validation error leaked endpoint credentials: %v", err)
	}
}
func TestTracingSecurityValidation(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "TLS below 1.2",
			config: Config{
				TLSConfig: &tls.Config{MinVersion: tls.VersionTLS11},
			},
		},
		{
			name: "uppercase metadata key",
			config: Config{
				Headers: map[string]string{"Authorization": "Bearer token"},
			},
		},
		{
			name: "reserved gRPC metadata key",
			config: Config{
				Headers: map[string]string{"grpc-timeout": "1S"},
			},
		},
		{
			name: "control character in metadata value",
			config: Config{
				Headers: map[string]string{"authorization": "Bearer token\x00"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.config.Enabled = true
			test.config.Endpoint = "telemetry.example.com:4317"
			if err := Validate(test.config); err == nil {
				t.Fatal("Validate() error = nil, want security validation error")
			}
		})
	}
}
