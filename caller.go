package goslogx

import (
	"github.com/muhammadluth/goslogx/internal/caller"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"go.uber.org/zap/zapcore"
)

const (
	maxCallerDepth    = 15
	defaultCallerSkip = 2
)

var internalPackages = []string{
	"github.com/muhammadluth/goslogx.",
	"/goslogx.",
}

// EncodeCaller writes an import-qualified source location. Zap's short caller
// encoder removes the module path, which makes logs ambiguous across services.
func EncodeCaller(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	if !caller.Defined {
		enc.AppendString("unknown:0")
		return
	}
	if source := callerSource(caller); source != "" {
		enc.AppendString(source)
		return
	}
	enc.AppendString(filepath.ToSlash(caller.File) + ":" + strconv.Itoa(caller.Line))
}

func callerSource(caller zapcore.EntryCaller) string {
	if caller.PC == 0 || caller.File == "" {
		return ""
	}
	fn := runtime.FuncForPC(caller.PC)
	if fn == nil {
		return ""
	}
	name := fn.Name()
	packagePath := name
	if index := strings.Index(packagePath, ".("); index >= 0 {
		packagePath = packagePath[:index]
	} else if index := strings.LastIndex(packagePath, "."); index >= 0 {
		packagePath = packagePath[:index]
	} else {
		return ""
	}
	return packagePath + "/" + filepath.Base(caller.File) + ":" + strconv.Itoa(caller.Line)
}

// detectCallerSkip finds the first caller outside goslogx wrappers
// and returns the correct skip value for zap.AddCallerSkip().
func detectCallerSkip() int {
	_, depth, ok := caller.Find(maxCallerDepth-1, func(frame runtime.Frame) bool {
		return frame.Function != "" && !isInternalPackage(frame.Function)
	})
	if ok {
		// zap.AddCallerSkip starts from the logger call.
		return depth - 1
	}
	return defaultCallerSkip
}

func isInternalPackage(funcName string) bool {
	for _, pkg := range internalPackages {
		if strings.Contains(funcName, pkg) {
			return true
		}
	}
	return false
}
