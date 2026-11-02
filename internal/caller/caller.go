// Package caller finds a useful caller frame without relying on a fixed stack depth.
package caller

import "runtime"

// Find returns the first frame accepted by keep. Depth is one-based relative to
// the caller of Find: depth 1 is that caller's caller.
func Find(maxDepth int, keep func(runtime.Frame) bool) (runtime.Frame, int, bool) {
	if maxDepth <= 0 {
		return runtime.Frame{}, 0, false
	}
	pcs := make([]uintptr, maxDepth)
	n := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for depth := 1; depth <= maxDepth; depth++ {
		frame, more := frames.Next()
		if keep(frame) {
			return frame, depth, true
		}
		if !more {
			break
		}
	}
	return runtime.Frame{}, 0, false
}
