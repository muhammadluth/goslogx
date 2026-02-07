package goslogx

import (
	"runtime"
	"strings"
)

// detectCallerSkip dynamically detects the correct caller skip level
// by finding the first caller outside the goslogx package.
// This allows the logger to correctly report the source location
// regardless of how many wrapper functions are used.
func detectCallerSkip() int {
	const maxDepth = 15 // Maximum call stack depth to search

	for i := 1; i < maxDepth; i++ {
		pc, _, _, ok := runtime.Caller(i)
		if !ok {
			break
		}

		// Get the function name to check the package
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}

		funcName := fn.Name()

		// Skip if this is still within goslogx package
		// Check for both "goslogx." and "/goslogx." to handle different import paths
		if strings.Contains(funcName, "github.com/muhammadluth/goslogx.") ||
			strings.Contains(funcName, "/goslogx.") {
			continue
		}

		// Found the first caller outside goslogx package
		// Return i-1 because zap.AddCallerSkip counts from the logger call
		return i - 1
	}

	// Fallback to default skip if detection fails
	return 2
}
