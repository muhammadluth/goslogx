package goslogx

import (
	"io"
	"os"

	"go.uber.org/zap/zapcore"
)

// Config holds logger configuration options.
type Config struct {
	// ServiceName is the name of the service using this logger.
	// It will be included in all log entries as "application_name".
	ServiceName string

	// Level is the minimum log level that will be output.
	// Default: zapcore.InfoLevel
	Level zapcore.Level

	// Output is the writer where logs will be written.
	// Default: os.Stdout
	Output io.Writer

	Debug bool

	// Masking controls automatic field masking behavior.
	Masking MaskingConfig
}

// MaskingConfig controls field masking behavior.
type MaskingConfig struct {
	// Enabled determines whether automatic field masking is active.
	// When true, struct fields tagged with log:"masked:*" will be masked.
	// Default: true
	Enabled bool
}

// Option configures a Logger.
type Option func(*Config)

// WithServiceName sets the service name for the logger.
// The service name appears in all log entries as "application_name".
//
// Example:
//
//	logger, _ := goslogx.New(goslogx.WithServiceName("my-service"))
func WithServiceName(name string) Option {
	return func(c *Config) {
		c.ServiceName = name
	}
}

// WithOutput sets the output writer for logs.
// By default, logs are written to os.Stdout.
//
// Example:
//
//	file, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
//	logger, _ := goslogx.New(
//	    goslogx.WithServiceName("my-service"),
//	    goslogx.WithOutput(file),
//	)
func WithOutput(w io.Writer) Option {
	return func(c *Config) {
		c.Output = w
	}
}

func WithDebug(debug bool) Option {
	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}
	return func(c *Config) {
		c.Level = level
	}
}

// WithMasking enables automatic field masking.
// When enabled, struct fields tagged with log:"masked:full" or log:"masked:partial"
// will be automatically masked in log output.
//
// Example:
//
//	logger, _ := goslogx.New(
//	    goslogx.WithServiceName("my-service"),
//	    goslogx.WithMasking(true),
//	)
func WithMasking(masking bool) Option {
	return func(c *Config) {
		c.Masking.Enabled = masking
	}
}

// defaultConfig returns the default logger configuration.
func defaultConfig() *Config {
	return &Config{
		ServiceName: "unknown",
		Level:       zapcore.InfoLevel,
		Output:      os.Stdout,
		Debug:       true,
		Masking: MaskingConfig{
			Enabled: true,
		},
	}
}
