package goslogx

import (
	"bytes"
	"crypto/tls"
	"os"
	"testing"
	"time"

	tracingimpl "github.com/muhammadluth/goslogx/internal/telemetry/tracing"

	"go.opentelemetry.io/otel/metric/noop"
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

	t.Run("WithMeterProvider", func(t *testing.T) {
		cfg := defaultConfig()
		provider := noop.NewMeterProvider()
		WithMeterProvider(provider)(cfg)
		if cfg.MeterProvider != provider {
			t.Fatal("MeterProvider was not stored")
		}
	})

	t.Run("WithInternalMetrics", func(t *testing.T) {
		cfg := defaultConfig()
		WithInternalMetrics(true)(cfg)
		if !cfg.InternalMetrics {
			t.Fatal("InternalMetrics was not enabled")
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
	if cfg.InternalMetrics {
		t.Error("Expected internal metrics to be disabled by default")
	}
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
	if options := tracingimpl.ExporterOptions(tracingimpl.Config(config.Tracing)); len(options) != 5 {
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
