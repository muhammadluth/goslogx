package goslogx

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var caller = runtime.Caller
var log Log
var once sync.Once

type Log struct {
	logger *zap.Logger
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

	// Create a core that writes to stdout, matching the previous behavior
	// where all used logging functions wrote to logger.stdout (os.Stdout).
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.Lock(os.Stdout),
		zapcore.DebugLevel, // Allow all levels
	)

	l := zap.New(core).With(zap.String("application_name", svcName))

	return Log{
		logger: l,
	}
}

func severityField(level zapcore.Level) zap.Field {
	var val string
	switch level {
	case zapcore.DebugLevel:
		val = "DEBUG"
	case zapcore.InfoLevel:
		val = "INFO"
	case zapcore.WarnLevel:
		val = "WARNING"
	case zapcore.ErrorLevel:
		val = "ERROR"
	case zapcore.FatalLevel: // Approximate for Error + 1
		val = "CRITICAL"
	default:
		val = "DEFAULT"
	}
	return zap.String("severity", val)
}

func getSourceField() zap.Field {
	pc, file, line, ok := caller(2)
	if !ok {
		return zap.Skip()
	}
	fn := runtime.FuncForPC(pc)
	function := "unknown"
	if fn != nil {
		function = fn.Name()
	}
	return zap.Any("source", map[string]any{
		"function": function,
		"file":     file,
		"line":     line,
	})
}

func Error(ctx context.Context, traceId string, module string, err error) {
	sLevel := zapcore.ErrorLevel
	caused := errors.Cause(err)
	stack := formatStack(err)
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.String("error", caused.Error()),
		severityField(sLevel),
		zap.String("stack_trace", stack),
		getSourceField(),
	}
	log.logger.Log(sLevel, err.Error(), fields...)
}

func Fatal(ctx context.Context, traceId string, module string, err error) {
	sLevel := zapcore.FatalLevel
	caused := errors.Cause(err)
	stack := formatStack(err)
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.String("error", caused.Error()),
		severityField(sLevel),
		zap.String("stack_trace", stack),
		getSourceField(),
	}
	log.logger.Log(sLevel, err.Error(), fields...)
}

func Warning(ctx context.Context, traceId string, module string, msg string, data any) {
	sLevel := zapcore.WarnLevel
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		severityField(sLevel),
	}
	if data != nil {
		fields = append(fields, zap.Any("data", data))
	}
	log.logger.Log(sLevel, msg, fields...)
}

func Info(ctx context.Context, traceId string, module string, msgType MsgType, msg string, data any) {
	sLevel := zapcore.InfoLevel
	fields := baseFields(traceId, module, msgType, sLevel, data)
	log.logger.Log(sLevel, msg, fields...)
}

func Debug(ctx context.Context, traceId string, module string, msgType MsgType, msg string, data any) {
	sLevel := zapcore.DebugLevel
	fields := baseFields(traceId, module, msgType, sLevel, data)
	log.logger.Log(sLevel, msg, fields...)
}

func baseFields(traceId string, module string, msgType MsgType, level zapcore.Level, data any) []zap.Field {
	fields := []zap.Field{
		zap.String("trace_id", traceId),
		zap.String("module", module),
		zap.String("msg_type", string(msgType)),
		severityField(level),
	}
	if data != nil {
		fields = append(fields, zap.Any("data", data))
	}
	return fields
}

func formatStack(err error) string {
	wrapped := errors.Wrap(err, "error occurred")
	stack := fmt.Sprintf("%+v", wrapped)
	stack = strings.ReplaceAll(stack, "\n\t", "")
	stack = strings.ReplaceAll(stack, "\n", " | ")
	return "[" + stack + "]"
}
