package goslogx

import (
	"bytes"
	"os"
	"testing"

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
		WithTracing()(cfg)
		if !cfg.Tracing.Enabled || cfg.Tracing.OTLP {
			t.Errorf("unexpected tracing config: %#v", cfg.Tracing)
		}
	})

	t.Run("WithOTLPTracing", func(t *testing.T) {
		cfg := defaultConfig()
		WithOTLPTracing("http://alloy:4317")(cfg)
		if !cfg.Tracing.Enabled || !cfg.Tracing.OTLP {
			t.Errorf("unexpected tracing config: %#v", cfg.Tracing)
		}
		if cfg.Tracing.Endpoint != "http://alloy:4317" {
			t.Errorf("Endpoint = %q", cfg.Tracing.Endpoint)
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
