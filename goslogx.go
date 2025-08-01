package goslogx

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

var logger Log
var once sync.Once

type Log struct {
	stdout *slog.Logger
	stderr *slog.Logger
}

func SetupLog(svcName string) {
	once.Do(func() {
		logger = NewLog(svcName)
	})
}

func NewLog(svcName string) Log {
	stdout := slog.New(slog.NewJSONHandler(os.Stdout, nil)).
		With("application_name", svcName)

	stderr := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
	})).With("application_name", svcName)
	return Log{
		stdout: stdout,
		stderr: stderr,
	}
}

func severityAttr(level slog.Level) slog.Attr {
	var val string
	switch level {
	case slog.LevelDebug:
		val = "DEBUG"
	case slog.LevelInfo:
		val = "INFO"
	case slog.LevelWarn:
		val = "WARNING"
	case slog.LevelError:
		val = "ERROR"
	case slog.LevelError + 1:
		val = "CRITICAL"
	default:
		val = "DEFAULT"
	}
	return slog.String("severity", val)
}

func getSourceAttr() slog.Attr {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		return slog.Attr{}
	}
	fn := runtime.FuncForPC(pc)
	function := "unknown"
	if fn != nil {
		function = fn.Name()
	}
	return slog.Any("source", map[string]any{
		"function": function,
		"file":     file,
		"line":     line,
	})
}

func Error(ctx context.Context, traceId string, module string, err error) {
	sLevel := slog.LevelError
	caused := errors.Cause(err)
	stack := formatStack(err)
	attrs := []slog.Attr{
		slog.String("trace_id", traceId),
		slog.String("module", module),
		slog.String("error", caused.Error()),
		severityAttr(sLevel),
		slog.String("stack_trace", stack),
		getSourceAttr(),
	}
	logger.stdout.LogAttrs(ctx, sLevel, err.Error(), attrs...)
}

func Fatal(ctx context.Context, traceId string, module string, err error) {
	sLevel := slog.LevelError + 1
	caused := errors.Cause(err)
	stack := formatStack(err)
	attrs := []slog.Attr{
		slog.String("trace_id", traceId),
		slog.String("module", module),
		slog.String("error", caused.Error()),
		severityAttr(sLevel),
		slog.String("stack_trace", stack),
		getSourceAttr(),
	}
	logger.stdout.LogAttrs(ctx, sLevel, err.Error(), attrs...)
	os.Exit(1)
}

func Warning(ctx context.Context, traceId string, module string, msg string, data any) {
	sLevel := slog.LevelWarn
	attrs := []slog.Attr{
		slog.String("trace_id", traceId),
		slog.String("module", module),
		severityAttr(sLevel),
	}
	if data != nil {
		attrs = append(attrs, slog.Any("data", data))
	}
	logger.stdout.LogAttrs(ctx, sLevel, msg, attrs...)
}

func Info(ctx context.Context, traceId string, module string, msgType string, msg string, data any) {
	sLevel := slog.LevelInfo
	attrs := baseAttrs(traceId, module, msgType, sLevel, data)
	logger.stdout.LogAttrs(ctx, sLevel, msg, attrs...)
}

func Debug(ctx context.Context, traceId string, module string, msgType string, msg string, data any) {
	sLevel := slog.LevelDebug
	attrs := baseAttrs(traceId, module, msgType, sLevel, data)
	logger.stdout.LogAttrs(ctx, sLevel, msg, attrs...)
}

func baseAttrs(traceId string, module string, msgType string, level slog.Level, data any) []slog.Attr {
	attrs := []slog.Attr{
		slog.String("trace_id", traceId),
		slog.String("module", module),
		slog.String("msg_type", msgType),
		severityAttr(level),
	}
	if data != nil {
		attrs = append(attrs, slog.Any("data", data))
	}
	return attrs
}

func formatStack(err error) string {
	wrapped := errors.Wrap(err, "error occurred")
	stack := fmt.Sprintf("%+v", wrapped)
	stack = strings.ReplaceAll(stack, "\n\t", "")
	stack = strings.ReplaceAll(stack, "\n", " | ")
	return "[" + stack + "]"
}
