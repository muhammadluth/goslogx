package goslogx

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

// TestSeverityField covers the switch cases in severityField, including default
func TestSeverityField(t *testing.T) {
	tests := []struct {
		level    zapcore.Level
		expected string
	}{
		{zapcore.DebugLevel, "DEBUG"},
		{zapcore.InfoLevel, "INFO"},
		{zapcore.WarnLevel, "WARNING"},
		{zapcore.ErrorLevel, "ERROR"},
		{zapcore.FatalLevel, "CRITICAL"},
		{zapcore.DPanicLevel, "DEFAULT"}, // Fallback case
		{zapcore.Level(100), "DEFAULT"},  // Unknown level
	}

	for _, tt := range tests {
		field := severityField(tt.level)
		if field.String != tt.expected {
			t.Errorf("severityField(%v) = %s, want %s", tt.level, field.String, tt.expected)
		}
	}
}

// TestGetSourceField covers getSourceField
func TestGetSourceField(t *testing.T) {
	// 1. Success case
	field := getSourceField()
	if field.Type == zapcore.SkipType {
		// Normal runtime shouldn't fail
	} else {
		m, ok := field.Interface.(map[string]any)
		if !ok {
			t.Errorf("Expected map[string]any interface, got %T", field.Interface)
		} else {
			if _, ok := m["function"]; !ok {
				t.Error("Expected function key in source map")
			}
		}
	}

	// 2. Failure case (mocking caller)
	originalCaller := caller
	defer func() { caller = originalCaller }()

	caller = func(skip int) (pc uintptr, file string, line int, ok bool) {
		return 0, "", 0, false
	}

	field = getSourceField()
	if field.Type != zapcore.SkipType {
		t.Errorf("Expected SkipType when caller fails, got %v", field.Type)
	}
}

// Helper to simulate runtime.Caller failure (mocking not possible for stdlib function easily without abstraction)
// But we can cover the nil function case if generic approach works.
// Actually, hard to match !ok from runtime.Caller in integration test.
// We accept that line 63 `if !ok { return zap.Skip() }` might remain uncovered unless we do deeper hacking.
// However, standard runs usually success.
// Let's check if there is anything else.
