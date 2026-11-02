package goslogx

import (
	"runtime"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestCallerSourceUsesImportQualifiedPath(t *testing.T) {
	pc, file, line, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	source := callerSource(zapcore.EntryCaller{PC: pc, File: file, Line: line, Defined: true})
	if !strings.HasPrefix(source, "github.com/muhammadluth/goslogx/") || !strings.HasSuffix(source, "/caller_encoder_test.go:"+strconv.Itoa(line)) {
		t.Fatalf("source = %q", source)
	}
}
