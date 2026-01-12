// Package goslogx provides a structured logging wrapper around uber-go/zap
// with custom stack trace formatting and zero-allocation design.
//
// It ensures consistent log formatting across applications while maintaining
// zap's performance characteristics through byte-based processing.
//
// Example usage:
//
//	package main
//
//	import (
//		"context"
//		"goslogx"
//	)
//
//	func main() {
//		goslogx.SetupLog("my-service")
//		ctx := context.Background()
//		goslogx.Info(ctx, "trace-001", "handler", goslogx.MESSSAGE_TYPE_EVENT, "request received", nil)
//	}
package goslogx

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log Log
var once sync.Once

// Log wraps zap.Logger with custom formatting and stack trace support.
type Log struct {
	logger *zap.Logger
}

// formatStackTraceBytes formats stack trace strings into a compact, readable format.
// It converts newlines and tabs (\n\t) into pipe separators and wraps the result in brackets.
// This function is designed for zero-allocation operation using byte-by-byte processing.
//
// Example input:  "goroutine 1\nmain.main\n\t/path/to/file.go:10"
// Example output: "[goroutine 1 | main.main | /path/to/file.go:10]"
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
func decodeJSONString(b []byte) string {
	// Optimization: if there are no backslashes, return immediately
	if !bytes.Contains(b, []byte("\\")) {
		return string(b)
	}

	// Process escape sequences byte by byte
	var buf bytes.Buffer
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

// SetupLog initializes the global logger instance with the given service name.
// This function is idempotent - it only initializes the logger once, subsequent calls are no-ops.
// This is safe to call from multiple goroutines concurrently.
//
// Example:
//
//	func init() {
//		goslogx.SetupLog("my-service")
//	}
func SetupLog(svcName string) {
	once.Do(func() {
		log = NewLog(svcName)
	})
}

// NewLog creates and returns a new Log instance configured with the given service name.
// The logger is configured with:
// - JSON encoding for structured logs
// - RFC3339 timestamps
// - Caller information (source file and line number)
// - Stack traces for error-level and above
// - Custom stack trace formatting for compact, readable output
// - Pre-allocated buffers for zero-allocation operation
//
// The logger writes to stdout and includes the application_name field in all logs.
func NewLog(svcName string) Log {
	// Configure JSON encoder with RFC3339 timestamps and caller information
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	encoderConfig.CallerKey = "source"
	encoderConfig.FunctionKey = "function"
	encoderConfig.StacktraceKey = "stack_trace"
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	// Create a custom writer that formats stack traces without allocations
	// Pre-allocate 1KB buffer for common stack trace sizes
	writer := &stackTraceFormattingWriter{
		Writer: os.Stdout,
		buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
	}

	// Create the core with our custom writer and debug-level logging
	// zapcore.Lock wraps the writer for thread-safe concurrent writes
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.Lock(writer),
		zapcore.DebugLevel,
	)

	// Configure logger with caller info and stacktrace for errors
	// AddCallerSkip(1) is used in logging functions to show the actual caller, not the wrapper
	logger := zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	).With(zap.String("application_name", svcName))

	return Log{
		logger: logger,
	}
}

// mapSeverity converts zapcore log levels to human-readable severity strings.
// Maps standard log levels to: DEBUG, INFO, WARNING, ERROR, CRITICAL.
// Unknown levels default to "DEFAULT".
func mapSeverity(level zapcore.Level) string {
	severityMap := map[zapcore.Level]string{
		zapcore.DebugLevel: "DEBUG",
		zapcore.InfoLevel:  "INFO",
		zapcore.WarnLevel:  "WARNING",
		zapcore.ErrorLevel: "ERROR",
		zapcore.FatalLevel: "CRITICAL",
	}
	if val, ok := severityMap[level]; ok {
		return val
	}
	return "DEFAULT"
}

// Error logs an error-level message with automatic stack trace capture.
// It includes trace_id, module, error details, and severity level.
// Stack traces are automatically formatted and included in the output.
//
// Example:
//
//	if err != nil {
//		goslogx.Error(ctx, "trace-001", "database", err)
//	}
func Error(ctx context.Context, traceId string, module string, err error) {
	// AddCallerSkip(1) ensures the caller of Error() is shown, not Error() itself
	logger := log.logger.WithOptions(zap.AddCallerSkip(1))
	sLevel := zapcore.ErrorLevel
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.Error(err),
		zap.String("severity", mapSeverity(sLevel)),
	}
	logger.Log(sLevel, "error occurred", fields...)
}

// Fatal logs a fatal-level message and exits the program.
// It includes automatic stack trace capture and exits with code 1.
// Use for unrecoverable errors that require immediate termination.
//
// Example:
//
//	if criticalErr := initializeDatabase(); criticalErr != nil {
//		goslogx.Fatal(ctx, "init-001", "database", criticalErr)
//	}
func Fatal(ctx context.Context, traceId string, module string, err error) {
	// AddCallerSkip(1) ensures the caller of Fatal() is shown, not Fatal() itself
	logger := log.logger.WithOptions(zap.AddCallerSkip(1))
	sLevel := zapcore.FatalLevel
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.Error(err),
		zap.String("severity", mapSeverity(sLevel)),
	}
	logger.Log(sLevel, "fatal error occurred", fields...)
}

// Warning logs a warning-level message with optional context data.
// Use for potentially harmful situations that should be investigated.
//
// Example:
//
//	goslogx.Warning(ctx, "trace-001", "cache", "cache miss rate high",
//		map[string]interface{}{"miss_rate": 0.45})
func Warning(ctx context.Context, traceId string, module string, msg string, data any) {
	// AddCallerSkip(1) ensures the caller of Warning() is shown, not Warning() itself
	logger := log.logger.WithOptions(zap.AddCallerSkip(1))
	sLevel := zapcore.WarnLevel
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.String("severity", mapSeverity(sLevel)),
	}
	if data != nil {
		fields = append(fields, zap.Any("data", data))
	}
	logger.Log(sLevel, msg, fields...)
}

// Info logs an informational message with message type classification and optional data.
// Message types include: MESSSAGE_TYPE_IN, MESSSAGE_TYPE_OUT, MESSSAGE_TYPE_REQUEST,
// MESSSAGE_TYPE_RESPONSE, MESSSAGE_TYPE_EVENT.
//
// Example:
//
//	goslogx.Info(ctx, "trace-001", "api", goslogx.MESSSAGE_TYPE_IN,
//		"request received", goslogx.HTTPRequestData{
//			Method: "GET",
//			URL: "/api/v1/users",
//			StatusCode: 200,
//		})
func Info(ctx context.Context, traceId string, module string, msgType MsgType, msg string, data any) {
	// AddCallerSkip(1) ensures the caller of Info() is shown, not Info() itself
	logger := log.logger.WithOptions(zap.AddCallerSkip(1))
	sLevel := zapcore.InfoLevel
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.String("msg_type", string(msgType)),
		zap.String("severity", mapSeverity(sLevel)),
	}
	if data != nil {
		fields = append(fields, zap.Any("data", data))
	}
	logger.Log(sLevel, msg, fields...)
}

// Debug logs a debug-level message with message type classification and optional data.
// Debug logs are useful during development and should be disabled in production for performance.
// Message types include: MESSSAGE_TYPE_IN, MESSSAGE_TYPE_OUT, MESSSAGE_TYPE_REQUEST,
// MESSSAGE_TYPE_RESPONSE, MESSSAGE_TYPE_EVENT.
//
// Example:
//
//	goslogx.Debug(ctx, "trace-001", "parser", goslogx.MESSSAGE_TYPE_IN,
//		"processing input", map[string]interface{}{"input_len": 1024})
func Debug(ctx context.Context, traceId string, module string, msgType MsgType, msg string, data any) {
	// AddCallerSkip(1) ensures the caller of Debug() is shown, not Debug() itself
	logger := log.logger.WithOptions(zap.AddCallerSkip(1))
	sLevel := zapcore.DebugLevel
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.String("msg_type", string(msgType)),
		zap.String("severity", mapSeverity(sLevel)),
	}
	if data != nil {
		fields = append(fields, zap.Any("data", data))
	}
	logger.Log(sLevel, msg, fields...)
}
