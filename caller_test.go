package goslogx_test

import (
	"errors"
	"testing"

	"github.com/muhammadluth/goslogx"
)

// Example: Direct call to goslogx.Error
// Source akan menunjuk ke baris ini
func TestDirectCall(t *testing.T) {
	goslogx.Error("trace-001", "test", errors.New("direct call error"))
	// Output source: example_test.go:14
}

// Example: Single wrapper function
// Source akan menunjuk ke baris pemanggilan LogError, bukan ke goslogx.Error
func LogError(traceID string, err error) {
	goslogx.Error(traceID, "wrapper", err)
}

func TestSingleWrapper(t *testing.T) {
	LogError("trace-002", errors.New("single wrapper error"))
	// Output source: example_test.go:24
}

// Example: Multiple nested wrappers
// Source akan menunjuk ke baris pemanggilan tertinggi
func LogErrorLevel1(traceID string, err error) {
	LogErrorLevel2(traceID, err)
}

func LogErrorLevel2(traceID string, err error) {
	LogErrorLevel3(traceID, err)
}

func LogErrorLevel3(traceID string, err error) {
	goslogx.Error(traceID, "nested-wrapper", err)
}

func TestNestedWrapper(t *testing.T) {
	LogErrorLevel1("trace-003", errors.New("nested wrapper error"))
	// Output source: example_test.go:43
}

// Example: Helper function dalam struct
type Service struct {
	name string
}

func (s *Service) ProcessData() error {
	err := errors.New("processing failed")
	s.logError("trace-004", err)
	return err
}

func (s *Service) logError(traceID string, err error) {
	goslogx.Error(traceID, s.name, err)
}

func TestStructMethod(t *testing.T) {
	svc := &Service{name: "user-service"}
	svc.ProcessData()
	// Output source: example_test.go:54 (dari ProcessData, bukan logError)
}

// Example: Testing all log levels
func TestAllLogLevels(t *testing.T) {
	// Info - tidak menggunakan caller detection
	goslogx.Info("trace-005", "test", goslogx.MESSSAGE_TYPE_EVENT, "info message", nil)

	// Warning - menggunakan caller detection
	goslogx.Warning("trace-006", "test", "warning message", nil)

	// Error - menggunakan caller detection
	goslogx.Error("trace-007", "test", errors.New("error message"))

	// Debug - menggunakan caller detection
	goslogx.Debug("trace-008", "test", goslogx.MESSSAGE_TYPE_EVENT, "debug message", nil)
}
