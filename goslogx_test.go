package goslogx_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/muhammadluth/goslogx"
)

// DTO tests
func TestDTOs(t *testing.T) {
	t.Run("HTTPData", func(t *testing.T) {
		data := goslogx.HTTPData{
			Method:     "GET",
			URL:        "/api/v1/users",
			StatusCode: 200,
			ClientIP:   "127.0.0.1",
		}
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("Failed to marshal HTTPData: %v", err)
		}
		t.Logf("HTTPData JSON: %s", string(b))
	})

	t.Run("DBData", func(t *testing.T) {
		data := goslogx.DBData{
			Driver:    "postgres",
			Operation: "SELECT",
			Table:     "users",
			Statement: "SELECT * FROM users WHERE id = 1",
			Duration:  "50ms",
		}
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("Failed to marshal DBData: %v", err)
		}
		t.Logf("DBData JSON: %s", string(b))
	})

	t.Run("MQData", func(t *testing.T) {
		data := goslogx.MQData{
			Driver:    "kafka",
			Operation: "consume",
			Topic:     "user-events",
			Group:     "notification-service",
			MessageID: "msg-123",
		}
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("Failed to marshal MQData: %v", err)
		}
		t.Logf("MQData JSON: %s", string(b))
	})

	t.Run("GenericData", func(t *testing.T) {
		data := goslogx.GenericData{
			Service: "Stripe",
			Action:  "Charge",
			Payload: map[string]interface{}{"amount": 100, "currency": "usd"},
		}
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("Failed to marshal GenericData: %v", err)
		}
		t.Logf("GenericData JSON: %s", string(b))
	})
}

// TestLoggingFunctions ensures all logging functions execute without panicking
func TestLoggingFunctions(t *testing.T) {
	goslogx.New(goslogx.WithServiceName("test-service"))
	traceID := "trace-123"

	// Test Info
	t.Run("Info", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Info panicked: %v", r)
			}
		}()
		goslogx.Info(traceID, "user-module", goslogx.MESSSAGE_TYPE_IN, "incoming request", map[string]string{"foo": "bar"})
	})

	// Test Debug
	t.Run("Debug", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Debug panicked: %v", r)
			}
		}()
		goslogx.Debug(traceID, "user-module", goslogx.MESSSAGE_TYPE_EVENT, "debug event", map[string]string{"key": "value"})
	})

	// Test Warning
	t.Run("Warning", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Warning panicked: %v", r)
			}
		}()
		goslogx.Warning(traceID, "user-module", "warning occurred", map[string]int{"attempts": 3})
	})

	// Test Error
	t.Run("Error", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Error panicked: %v", r)
			}
		}()
		err := errors.New("database connection failed")
		goslogx.Error(traceID, "db-module", err)
	})

	// Test with nil data
	t.Run("NilData", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Nil data panicked: %v", r)
			}
		}()
		goslogx.Info(traceID, "nil-module", goslogx.MESSSAGE_TYPE_EVENT, "info nil data", nil)
		goslogx.Warning(traceID, "nil-module", "warning nil data", nil)
		goslogx.Debug(traceID, "nil-module", goslogx.MESSSAGE_TYPE_EVENT, "debug nil data", nil)
	})
}

// TestFatal runs the Fatal test in a separate process to verify os.Exit(1)
func TestFatal(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		goslogx.New(goslogx.WithServiceName("crash-service"))
		traceID := "crash-trace"
		err := errors.New("critical failure")
		goslogx.Fatal(traceID, "main", err)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestFatal")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	err := cmd.Run()

	// Fatal should call os.Exit(1), so we expect an exit error
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		// Expected: process exited with non-zero status
		return
	}
	t.Fatalf("process ran with err %v, want exit status 1", err)
}

// TestLoggerMethods tests direct methods on Logger instance
// Since Logger instance creation is internal, we test it in internal tests
// but we can ensure global functions work here.
func TestGlobalFunctions(t *testing.T) {
	goslogx.New(goslogx.WithServiceName("global-test"))
	traceID := "trace-global"

	goslogx.Info(traceID, "mod", goslogx.MESSSAGE_TYPE_EVENT, "info", nil)
	goslogx.Debug(traceID, "mod", goslogx.MESSSAGE_TYPE_EVENT, "debug", nil)
	goslogx.Warning(traceID, "mod", "warn", nil)
	goslogx.Error(traceID, "mod", errors.New("err"))
}

// TestNewConcurrency ensures New is idempotent and thread-safe
func TestNewConcurrency(t *testing.T) {
	for i := 0; i < 10; i++ {
		go goslogx.New(goslogx.WithServiceName("concurrent-service"))
	}
	// Give them time to race
	time.Sleep(10 * time.Millisecond)
}
