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

type Log struct {
	logger *zap.Logger
}

// formatStackTraceBytes formats the stack trace in a compact way without allocations
func formatStackTraceBytes(dst *bytes.Buffer, stackStr string) {
	dst.WriteByte('[')
	for i := 0; i < len(stackStr); i++ {
		if i+2 <= len(stackStr) && stackStr[i] == '\n' && stackStr[i+1] == '\t' {
			dst.WriteString(" | ")
			i++ // skip the tab
			continue
		}
		if stackStr[i] == '\n' {
			dst.WriteString(" | ")
		} else {
			dst.WriteByte(stackStr[i])
		}
	}
	dst.WriteByte(']')
}

// stackTraceFormattingWriter wraps io.Writer to format stack traces in JSON output
type stackTraceFormattingWriter struct {
	io.Writer
	buf *bytes.Buffer // Reuse buffer to reduce allocations
}

// Write implements io.Writer and formats stack traces in JSON using zero-copy approach
func (w *stackTraceFormattingWriter) Write(p []byte) (n int, err error) {
	// Only process if there's a stack_trace field - use simple byte search to avoid allocations
	if !bytes.Contains(p, []byte("\"stack_trace\":\"")) {
		return w.Writer.Write(p)
	}

	// Find the position of "stack_trace":" and extract the stack value
	stackTraceKey := []byte("\"stack_trace\":\"")
	idx := bytes.Index(p, stackTraceKey)

	// Find the closing quote of the stack_trace value
	startIdx := idx + len(stackTraceKey)
	endIdx := startIdx
	for endIdx < len(p)-1 {
		if p[endIdx] == '\\' {
			endIdx += 2 // Skip escaped character
			continue
		}
		if p[endIdx] == '"' {
			break
		}
		endIdx++
	}

	if endIdx >= len(p) {
		return w.Writer.Write(p)
	}

	// Decode the escaped JSON string to get actual stack trace
	stackBytes := p[startIdx:endIdx]
	stackStr := decodeJSONString(stackBytes)

	// Format the stack trace and write the result
	w.buf.Reset()
	w.buf.Write(p[:startIdx])
	formatStackTraceBytes(w.buf, stackStr)
	w.buf.Write(p[endIdx:]) // closing quote and rest

	return w.Writer.Write(w.buf.Bytes())
}

// decodeJSONString decodes an escaped JSON string
func decodeJSONString(b []byte) string {
	// For most cases, there are no escapes, so we can return directly
	if !bytes.Contains(b, []byte("\\")) {
		return string(b)
	}
	// If there are escapes, we need to process them
	var buf bytes.Buffer
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+1 < len(b) {
			next := b[i+1]
			switch next {
			case '"':
				buf.WriteByte('"')
				i++
			case '\\':
				buf.WriteByte('\\')
				i++
			case 'n':
				buf.WriteByte('\n')
				i++
			case 't':
				buf.WriteByte('\t')
				i++
			default:
				buf.WriteByte(b[i])
			}
		} else {
			buf.WriteByte(b[i])
		}
	}
	return buf.String()
}

// Sync implements zapcore.WriteSyncer
func (w *stackTraceFormattingWriter) Sync() error {
	if syncer, ok := w.Writer.(zapcore.WriteSyncer); ok {
		return syncer.Sync()
	}
	return nil
}

func SetupLog(svcName string) {
	once.Do(func() {
		log = NewLog(svcName)
	})
}

func NewLog(svcName string) Log {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	encoderConfig.CallerKey = "source"
	encoderConfig.FunctionKey = "function"
	encoderConfig.StacktraceKey = "stack_trace"
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	// Create a writer that formats stack traces with pre-allocated buffer
	writer := &stackTraceFormattingWriter{
		Writer: os.Stdout,
		buf:    bytes.NewBuffer(make([]byte, 0, 1024)), // Pre-allocate 1KB buffer
	}

	// Create a core that writes to our custom writer with caller and stacktrace enabled
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.Lock(writer),
		zapcore.DebugLevel,
	)

	// Enable caller info and stacktrace for error level
	logger := zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	).With(zap.String("application_name", svcName))

	return Log{
		logger: logger,
	}
}

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

func Error(ctx context.Context, traceId string, module string, err error) {
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

func Fatal(ctx context.Context, traceId string, module string, err error) {
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

func Warning(ctx context.Context, traceId string, module string, msg string, data any) {
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

func Info(ctx context.Context, traceId string, module string, msgType MsgType, msg string, data any) {
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

func Debug(ctx context.Context, traceId string, module string, msgType MsgType, msg string, data any) {
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
