package goslogx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/muhammadluth/goslogx"
	"github.com/pkg/errors"
)

// DTO tests
func TestDTOs(t *testing.T) {
	t.Run("HTTPRequestData", func(t *testing.T) {
		data := goslogx.HTTPRequestData{
			Method:     "GET",
			URL:        "/api/v1/users",
			StatusCode: 200,
			ClientIP:   "127.0.0.1",
		}
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("Failed to marshal HTTPRequestData: %v", err)
		}
		t.Logf("HTTPRequestData JSON: %s", string(b))
	})

	t.Run("DBData", func(t *testing.T) {
		data := goslogx.DBData{
			Driver:     "postgres",
			Operation:  "SELECT",
			Table:      "users",
			Statement:  "SELECT * FROM users WHERE id = 1",
			DurationMs: 50,
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

// Log functions tests
func TestLoggingFunctions(t *testing.T) {
	// We need to capture stdout.
	// Since goslogx uses os.Stdout directly, we need to swap os.Stdout.
	// IMPORTANT: This works only if SetupLog hasn't been called yet with the original stdout,
	// or if we can force re-init. But goslogx has sync.Once.
	// So we must rely on this test running in a fresh process or being the first to call SetupLog.
	// However, `go test` runs all tests in the same process.
	// We'll wrap the actual logging tests in a generic function and run it.

	// Pipe to capture stdout
	r, w, _ := os.Pipe()
	originalStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = originalStdout
	}()

	// Initialize the logger (this will pick up the pipe writer as stdout)
	goslogx.SetupLog("test-service")

	ctx := context.Background()
	traceID := "trace-123"

	// 1. Test Info
	t.Run("Info", func(t *testing.T) {
		goslogx.Info(ctx, traceID, "user-module", goslogx.MESSSAGE_TYPE_IN, "incoming request", map[string]string{"foo": "bar"})
	})

	// 2. Test Debug
	t.Run("Debug", func(t *testing.T) {
		goslogx.Debug(ctx, traceID, "user-module", goslogx.MESSSAGE_TYPE_EVENT, "debug event", map[string]string{"key": "value"})
	})

	// 3. Test Warning
	t.Run("Warning", func(t *testing.T) {
		goslogx.Warning(ctx, traceID, "user-module", "warning occurred", map[string]int{"attempts": 3})
	})

	// 4. Test Error
	t.Run("Error", func(t *testing.T) {
		err := errors.New("database connection failed")
		goslogx.Error(ctx, traceID, "db-module", err)
	})

	// Close writer to finish capturing
	w.Close()

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Validation
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 4 {
		t.Errorf("Expected at least 4 log lines, got %d", len(lines))
	}

	for _, line := range lines {
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			t.Errorf("Failed to unmarshal log line: %s", line)
			continue
		}

		if logEntry["trace_id"] != traceID {
			t.Errorf("Expected trace_id %s, got %v", traceID, logEntry["trace_id"])
		}
		if logEntry["application_name"] != "test-service" {
			t.Errorf("Expected application_name test-service, got %v", logEntry["application_name"])
		}
	}
}

// TestFatal runs the Fatal test in a separate process to verify os.Exit(1)
func TestFatal(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		goslogx.SetupLog("crash-service")
		ctx := context.Background()
		traceID := "crash-trace"
		err := errors.New("critical failure")
		goslogx.Fatal(ctx, traceID, "main", err)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestFatal")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		// Verify output contained the log
		output := stdout.String()
		if !strings.Contains(output, "critical failure") {
			t.Errorf("Expected output to contain 'critical failure', got: %s", output)
		}
		if !strings.Contains(output, "CRITICAL") {
			t.Errorf("Expected severity CRITICAL, got: %s", output)
		}
		return
	}
	t.Fatalf("process ran with err %v, want exit status 1", err)
}

// TestSetupLogConcurrency ensures SetupLog is idempotent and thread-safe
func TestSetupLogConcurrency(t *testing.T) {
	for i := 0; i < 10; i++ {
		go goslogx.SetupLog("concurrent-service")
	}
	// Give them time to race
	time.Sleep(10 * time.Millisecond)
	// If we didn't panic, we're good (sync.Once covers this, but good to cover line)
}
