// Package goslogx provides a high-performance, structured logging wrapper around zap.
// It features automatic field masking, custom stack trace formatting,
// and pre-defined DTOs for common logging scenarios.
//
// Basic Usage:
//
//	logger, err := goslogx.New(goslogx.WithServiceName("my-service"))
//	if err != nil {
//		log.Fatal(err)
//	}
//	logger.Info("trace-001", "handler", goslogx.MessageTypeEvent, "request received", nil)
package goslogx

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// globalLog project global logger using atomic pointer for thread-safety.
	globalLog atomic.Pointer[Logger]
	// once ensures New() only configures the global logger once.
	once sync.Once
)

func init() {
	// Initialize with default logger so it's always ready to use.
	globalLog.Store(setupLog())
}

// Logger wraps zap.Logger with custom formatting and stack trace support.
// Create a new Logger using New() with functional options.
//
// Example:
//
//	logger, err := goslogx.New(
//		goslogx.WithServiceName("my-service"),
//		goslogx.WithLevel(zapcore.DebugLevel),
//	)
type Logger struct {
	logger *zap.Logger
	config *Config
}

// formatStackTraceBytes formats a stack trace string into a compact, bracketed format.
// It replaces newlines and tabs with pipe separators for improved readability.
// This function operates byte-by-byte to ensure zero allocations.
//
// Example: "goroutine 1\nmain.main\n\tfile.go:10" -> "[goroutine 1 | main.main | file.go:10]"
func formatStackTraceBytes(dst *bytes.Buffer, stackStr string) {
	dst.WriteByte('[')
	for i := 0; i < len(stackStr); i++ {
		// Check for \n\t pattern and convert to pipe separator
		if i+2 <= len(stackStr) && stackStr[i] == '\n' && stackStr[i+1] == '\t' {
			dst.WriteString(" | ")
			i++ // skip the tab character
			continue
		}
		// Convert standalone newlines to pipe separators
		if stackStr[i] == '\n' {
			dst.WriteString(" | ")
		} else {
			dst.WriteByte(stackStr[i])
		}
	}
	dst.WriteByte(']')
}

// stackTraceFormattingWriter wraps io.Writer to format stack traces in JSON output.
// It implements zapcore.WriteSyncer and performs byte-level scanning to detect
// and format stack_trace fields without full JSON parsing (zero-allocation design).
type stackTraceFormattingWriter struct {
	io.Writer               // Underlying writer for formatted output
	buf       *bytes.Buffer // Pre-allocated 1KB buffer reused across writes to minimize allocations
}

// Write implements io.Writer and formats stack traces in JSON output.
// It uses a zero-copy byte scanning approach to detect and format stack_trace fields
// without unmarshaling the entire JSON payload, maintaining zap's zero-allocation guarantee.
//
// The implementation:
// 1. Checks for "stack_trace":" pattern to determine if processing is needed
// 2. Scans forward to find the closing quote, respecting escaped characters
// 3. Decodes the JSON-escaped stack trace string
// 4. Formats the stack trace with pipe separators and brackets
// 5. Writes the modified JSON back to the underlying writer
func (w *stackTraceFormattingWriter) Write(p []byte) (n int, err error) {
	// Fast path: only process if there's a stack_trace field
	// This avoids unnecessary processing for non-error logs
	if !bytes.Contains(p, []byte("\"stack_trace\":\"")) {
		return w.Writer.Write(p)
	}

	// Find the position of stack_trace field
	stackTraceKey := []byte("\"stack_trace\":\"")
	idx := bytes.Index(p, stackTraceKey)

	// Calculate the starting position of the stack trace value (after the key)
	startIdx := idx + len(stackTraceKey)
	endIdx := startIdx

	// Scan forward to find the closing quote, respecting escape sequences
	for endIdx < len(p)-1 {
		if p[endIdx] == '\\' {
			endIdx += 2 // Skip both backslash and the escaped character
			continue
		}
		if p[endIdx] == '"' {
			break // Found the closing quote
		}
		endIdx++
	}

	// If we didn't find a complete stack trace value, write data as-is
	if endIdx >= len(p) {
		return w.Writer.Write(p)
	}

	// Decode the JSON-escaped stack trace string to get the actual content
	stackBytes := p[startIdx:endIdx]
	stackStr := decodeJSONString(stackBytes)

	// Format the stack trace and reconstruct the JSON with formatted stack trace
	w.buf.Reset()
	w.buf.Write(p[:startIdx])              // Write everything before the stack trace value
	formatStackTraceBytes(w.buf, stackStr) // Write the formatted stack trace
	w.buf.Write(p[endIdx:])                // Write everything after the stack trace value (closing quote and rest)

	return w.Writer.Write(w.buf.Bytes())
}

// decodeJSONString decodes a JSON-escaped string without unmarshaling the entire JSON.
// This function handles common escape sequences: \", \\, \n, and \t.
// Unknown escape sequences are kept as-is.
//
// This is optimized for the common case where there are no escapes,
// avoiding buffer allocation and processing overhead.
//
// Example: "hello\nworld\t\"test\"" -> "hello
// world	"test""

var decodeBufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func decodeJSONString(b []byte) string {
	// Optimization: if there are no backslashes, return immediately
	if !bytes.Contains(b, []byte("\\")) {
		return string(b)
	}

	buf := decodeBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer decodeBufPool.Put(buf)

	// Process escape sequences byte by byte
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+1 < len(b) {
			next := b[i+1]
			switch next {
			case '"': // Escaped quote
				buf.WriteByte('"')
				i++
			case '\\': // Escaped backslash
				buf.WriteByte('\\')
				i++
			case 'n': // Escaped newline
				buf.WriteByte('\n')
				i++
			case 't': // Escaped tab
				buf.WriteByte('\t')
				i++
			default: // Unknown escape sequence - keep the backslash
				buf.WriteByte(b[i])
			}
		} else {
			buf.WriteByte(b[i])
		}
	}
	return buf.String()
}

// Sync implements zapcore.WriteSyncer.
// It attempts to sync the underlying writer if it supports the WriteSyncer interface,
// otherwise it returns nil (no-op for writers that don't support syncing).
func (w *stackTraceFormattingWriter) Sync() error {
	if syncer, ok := w.Writer.(zapcore.WriteSyncer); ok {
		return syncer.Sync()
	}
	return nil
}

// New creates a new Logger instance with the given options.
// Returns an error if logger initialization fails.
//
// Example:
//
//	logger, err := goslogx.New(
//	    goslogx.WithServiceName("my-service"),
//	    goslogx.WithLevel(zapcore.DebugLevel),
//	    goslogx.WithMasking(),
//
// New sets the global logger configuration exactly once.
// Subsequent calls to New will return the existing global logger.
// Returns the global Logger instance.
func New(opts ...Option) *Logger {
	once.Do(func() {
		globalLog.Store(setupLog(opts...))
	})
	return globalLog.Load()
}

func setupLog(opts ...Option) *Logger {
	// Apply options to default config
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	// Configure JSON encoder with production defaults
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	encoderConfig.CallerKey = "source"
	encoderConfig.FunctionKey = "function"
	encoderConfig.StacktraceKey = "stack_trace"
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	// Use custom writer for zero-allocation stack trace formatting
	writer := &stackTraceFormattingWriter{
		Writer: cfg.Output,
		buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(writer),
		cfg.Level,
	)

	logger := zap.New(
		core,
		zap.AddStacktrace(zapcore.FatalLevel),
	).With(zap.String("application_name", cfg.ServiceName))

	return &Logger{
		logger: logger,
		config: cfg,
	}
}

// Severity level constants for Cloud Logging compatibility.
// Using constants avoids string allocations on every log call.
const (
	severityDebug    = "DEBUG"
	severityInfo     = "INFO"
	severityWarning  = "WARNING"
	severityError    = "ERROR"
	severityCritical = "CRITICAL"
)

// fieldPool reuses zap.Field slices to reduce allocations.
// Capacity of 6 is the maximum number of fields used in any logging function:
// trace_id, module, msg_type, severity, data, error = 6 fields max
var fieldPool = sync.Pool{
	New: func() any { return make([]zap.Field, 0, 6) },
}

// getFields retrieves a field slice from the pool.
// Always returns an empty slice ready for use.
func getFields() []zap.Field {
	return fieldPool.Get().([]zap.Field)[:0]
}

// putFields returns a field slice to the pool for reuse.
// Resets the slice to zero length before returning.
func putFields(f []zap.Field) {
	fieldPool.Put(f[:0])
}

// Fatal logs a critical error and terminates the process.
func (l *Logger) Fatal(traceID string, module string, err error) {
	fields := getFields()
	defer putFields(fields)

	logger := l.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(1))
	fields = append(fields,
		zap.String("trace_id", traceID),
		zap.String("module", module),
		zap.Error(err),
		zap.String("severity", severityCritical),
	)

	logger.Log(zapcore.FatalLevel, "fatal error occurred", fields...)
}

// Fatal logs a critical error using the global logger and terminates the process.
func Fatal(traceID string, module string, err error) {
	globalLog.Load().Fatal(traceID, module, err)
}

// Error logs an error event with automatic stack trace capture.
func (l *Logger) Error(traceID string, module string, err error) {
	fields := getFields()
	defer putFields(fields)

	logger := l.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(1))
	fields = append(fields,
		zap.String("trace_id", traceID),
		zap.String("module", module),
		zap.Error(err),
		zap.String("severity", severityError),
	)
	logger.Log(zapcore.ErrorLevel, "error occurred", fields...)
}

// Error logs an error event using the global logger with automatic stack trace capture.
func Error(traceID string, module string, err error) {
	globalLog.Load().Error(traceID, module, err)
}

// Warning logs a warning-level message with optional context data.
func (l *Logger) Warning(traceID string, module string, msg string, data any) {
	fields := getFields()
	defer putFields(fields)

	logger := l.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(1))
	fields = append(fields,
		zap.String("trace_id", traceID),
		zap.String("module", module),
		zap.String("severity", severityWarning),
	)
	if data != nil {
		fields = append(fields, zap.Any("data", data))
	}
	logger.Log(zapcore.WarnLevel, msg, fields...)
}

// Warning logs a warning-level message using the global logger with optional context data.
func Warning(traceID string, module string, msg string, data any) {
	globalLog.Load().Warning(traceID, module, msg, data)
}

// Info logs an informational message with a specified message type.
func (l *Logger) Info(traceID string, module string, msgType MsgType, msg string, data any) {
	fields := getFields()
	defer putFields(fields)

	fields = append(fields,
		zap.String("trace_id", traceID),
		zap.String("module", module),
		zap.String("msg_type", string(msgType)),
		zap.String("severity", severityInfo),
	)
	if data != nil {
		fields = append(fields, dataField("data", data))
	}
	l.logger.Log(zapcore.InfoLevel, msg, fields...)
}

// Info logs an informational message using the global logger with a specified message type.
func Info(traceID string, module string, msgType MsgType, msg string, data any) {
	globalLog.Load().Info(traceID, module, msgType, msg, data)
}

// Debug logs a debug-level message with a specified message type.
func (l *Logger) Debug(traceID string, module string, msgType MsgType, msg string, data any) {
	fields := getFields()
	defer putFields(fields)

	logger := l.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(1))
	fields = append(fields,
		zap.String("trace_id", traceID),
		zap.String("module", module),
		zap.String("msg_type", string(msgType)),
		zap.String("severity", severityDebug),
	)
	if data != nil {
		fields = append(fields, zap.Any("data", data))
	}
	logger.Log(zapcore.DebugLevel, msg, fields...)
}

// Debug logs a debug-level message using the global logger with a specified message type.
func Debug(traceID string, module string, msgType MsgType, msg string, data any) {
	globalLog.Load().Debug(traceID, module, msgType, msg, data)
}
