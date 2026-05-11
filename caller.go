package goslogx

import (
	"runtime"
	"strings"
)

const (
	maxCallerDepth    = 15
	defaultCallerSkip = 2
)

var internalPackages = []string{
	"github.com/muhammadluth/goslogx.",
	"/goslogx.",
}

// detectCallerSkip finds the first caller outside goslogx wrappers
// and returns the correct skip value for zap.AddCallerSkip().
func detectCallerSkip() int {
	for depth := 1; depth < maxCallerDepth; depth++ {
		pc, _, _, ok := runtime.Caller(depth)
		if !ok {
			break
		}

		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}

		funcName := fn.Name()

		if isInternalPackage(funcName) {
			continue
		}

		// runtime.Caller() includes this function itself,
		// while zap.AddCallerSkip() starts from the logger call.
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
