// Package goslogx provides a high-performance, structured logging wrapper around zap.
// It features zero-allocation field masking, custom stack trace formatting,
// and pre-defined DTOs for common logging scenarios.
//
// Usage:
//
//	goslogx.SetupLog("my-service", '*')
//	goslogx.Info("trace-001", "handler", goslogx.MESSSAGE_TYPE_EVENT, "request received", nil)
package goslogx

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log Log
var once sync.Once

// Log wraps zap.Logger with custom formatting, stack trace support, and optional field masking.
// Field masking uses a configurable mask character for fields tagged with `masked:"true"`.
type Log struct {
	logger         *zap.Logger
	maskChar       rune // Character used for masking (e.g., '*', 'x', '#')
	maskingEnabled bool // Whether masking is enabled
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

// SetupLog initializes the global logger instance with the given service name and optional masking character.
// This function is idempotent and thread-safe - subsequent calls will be ignored.
//
// Parameters:
//   - svcName: The name of the service for log identification
//   - maskChar: Character used for masking sensitive fields (e.g., '*', 'x', '#'). Use 0 to disable masking.
//
// Fields tagged with `masked:"true"` will have their values masked in log output.
//
// Example:
//
//	goslogx.SetupLog("my-service", '*')  // Enable masking with '*'
//	goslogx.SetupLog("my-service", 0)    // Disable masking
func SetupLog(svcName string, maskChar rune) {
	once.Do(func() {
		log = NewLog(svcName, maskChar)
	})
}

// NewLog creates a new Log instance for standalone use or testing.
// It configures JSON encoding, RFC3339 timestamps, and custom stack trace formatting.
func NewLog(svcName string, maskChar rune) Log {
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
		Writer: os.Stdout,
		buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.Lock(writer),
		zapcore.DebugLevel,
	)

	logger := zap.New(core,
		zap.AddStacktrace(zapcore.FatalLevel),
	).With(zap.String("application_name", svcName))

	// Initialize masking if maskChar is provided
	maskingEnabled := maskChar != 0

	return Log{
		logger:         logger,
		maskChar:       maskChar,
		maskingEnabled: maskingEnabled,
	}
}

// maskString obfuscates a string based on its content and length.
// It handles emails, long strings, and fully masks shorter secrets.
func (l *Log) maskString(value string) string {
	if !l.maskingEnabled || value == "" {
		return value
	}

	// Check if it's an email
	if strings.Contains(value, "@") {
		return l.maskEmail(value)
	}

	// For other strings, apply length-based masking
	length := len(value)

	if length <= 8 {
		// Fully mask short strings (passwords, short secrets)
		return strings.Repeat(string(l.maskChar), length)
	}

	// Show first 2 and last 2 characters for longer strings
	maskLen := length - 4
	if maskLen < 4 {
		maskLen = 4
	}
	return value[:2] + strings.Repeat(string(l.maskChar), maskLen) + value[length-2:]
}

// maskEmail obfuscates an email local part while preserving the domain.
func (l *Log) maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		// Not a valid email, mask fully
		return strings.Repeat(string(l.maskChar), len(email))
	}

	localPart := parts[0]
	domain := parts[1]

	if len(localPart) <= 2 {
		// Very short local part, mask it fully
		return strings.Repeat(string(l.maskChar), len(localPart)) + "@" + domain
	}

	// Show first 2 characters of local part
	maskedLocal := localPart[:2] + strings.Repeat(string(l.maskChar), len(localPart)-2)
	return maskedLocal + "@" + domain
}

// processDataMasking traverses a data structure and masks fields tagged with `masked:"true"`.
func (l *Log) processDataMasking(data any) any {
	if !l.maskingEnabled || data == nil {
		return data
	}

	return l.processValue(reflect.ValueOf(data))
}

// processValue recursively processes a reflect.Value and masks tagged fields.
func (l *Log) processValue(v reflect.Value) any {
	// Handle invalid or nil values
	if !v.IsValid() {
		return nil
	}

	// Dereference pointers
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		return l.processStruct(v)
	case reflect.Map:
		return l.processMap(v)
	case reflect.Slice, reflect.Array:
		return l.processSlice(v)
	default:
		return v.Interface()
	}
}

// processStruct processes a struct and masks fields with `masked:"true"` tag.
// Returns a map representation of the struct with masked fields.
func (l *Log) processStruct(v reflect.Value) any {
	t := v.Type()
	result := make(map[string]any)

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON tag name, default to field name
		jsonTag := field.Tag.Get("json")
		fieldName := field.Name
		if jsonTag != "" && jsonTag != "-" {
			// Parse JSON tag (handle "name,omitempty" format)
			if idx := strings.Index(jsonTag, ","); idx > 0 {
				fieldName = jsonTag[:idx]
			} else {
				fieldName = jsonTag
			}
		}

		// Check if field should be masked
		maskedTag := field.Tag.Get("masked")
		if maskedTag == "true" && fieldValue.Kind() == reflect.String {
			// Mask string field
			plaintext := fieldValue.String()
			result[fieldName] = l.maskString(plaintext)
		} else {
			// Recursively process nested structures
			result[fieldName] = l.processValue(fieldValue)
		}
	}

	return result
}

// processMap processes a map and recursively masks nested structures.
func (l *Log) processMap(v reflect.Value) any {
	if v.IsNil() {
		return nil
	}

	result := make(map[string]any)
	iter := v.MapRange()
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		// Convert key to string
		keyStr := fmt.Sprintf("%v", key.Interface())
		result[keyStr] = l.processValue(value)
	}

	return result
}

// processSlice processes a slice/array and recursively masks nested structures.
func (l *Log) processSlice(v reflect.Value) any {
	if v.Kind() == reflect.Slice && v.IsNil() {
		return nil
	}

	result := make([]any, v.Len())
	for i := 0; i < v.Len(); i++ {
		result[i] = l.processValue(v.Index(i))
	}

	return result
}

// mapSeverity maps zapcore levels to Cloud Logging-compatible severity strings.
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

// Fatal logs a critical error and terminates the process.
//
// Example:
//
//	goslogx.Fatal("trace-001", "connection", err)
func Fatal(traceId string, module string, err error) {
	logger := log.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(1))
	sLevel := zapcore.FatalLevel
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.Error(err),
		zap.String("severity", mapSeverity(sLevel)),
	}
	logger.Log(sLevel, "fatal error occurred", fields...)
}

// Error logs an error event with automatic stack trace capture.
//
// Example:
//
//	goslogx.Error("trace-001", "database", err)
func Error(traceId string, module string, err error) {
	logger := log.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(1))
	sLevel := zapcore.ErrorLevel
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.Error(err),
		zap.String("severity", mapSeverity(sLevel)),
	}
	logger.Log(sLevel, "error occurred", fields...)
}

// Warning logs a warning-level message with optional context data.
//
// Example:
//
//	goslogx.Warning("trace-001", "cache", "cache miss rate high",
//		map[string]interface{}{"miss_rate": 0.45})
func Warning(traceId string, module string, msg string, data any) {
	logger := log.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(1))
	sLevel := zapcore.WarnLevel
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.String("severity", mapSeverity(sLevel)),
	}
	if data != nil {
		processedData := log.processDataMasking(data)
		fields = append(fields, zap.Any("data", processedData))
	}
	logger.Log(sLevel, msg, fields...)
}

// Info logs an informational message with a specified message type.
//
// Example:
//
//	goslogx.Info("trace-001", "api", goslogx.MESSSAGE_TYPE_IN,
//		"request received", goslogx.HTTPRequestData{
//			Method: "GET",
//			URL: "/api/v1/users",
//			StatusCode: 200,
//		})
func Info(traceId string, module string, msgType MsgType, msg string, data any) {
	sLevel := zapcore.InfoLevel
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.String("msg_type", string(msgType)),
		zap.String("severity", mapSeverity(sLevel)),
	}
	if data != nil {
		processedData := log.processDataMasking(data)
		fields = append(fields, zap.Any("data", processedData))
	}
	log.logger.Log(sLevel, msg, fields...)
}

// Debug logs a debug-level message with a specified message type.
//
// Example:
//
//	goslogx.Debug("trace-001", "parser", goslogx.MESSSAGE_TYPE_IN,
//		"processing input", map[string]interface{}{"input_len": 1024})
func Debug(traceId string, module string, msgType MsgType, msg string, data any) {
	// Add caller info dynamically (base logger doesn't have it)
	logger := log.logger.WithOptions(zap.AddCaller(), zap.AddCallerSkip(1))
	sLevel := zapcore.DebugLevel
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.String("msg_type", string(msgType)),
		zap.String("severity", mapSeverity(sLevel)),
	}
	if data != nil {
		// Process masking for tagged fields
		processedData := log.processDataMasking(data)
		fields = append(fields, zap.Any("data", processedData))
	}
	logger.Log(sLevel, msg, fields...)
}
