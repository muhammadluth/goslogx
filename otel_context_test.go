package goslogx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

const (
	testTraceID = "c157bd358d20626a78eb4182fb353753"
	testSpanID  = "6e642e0aa213d5ab"
)

func contextWithTestSpan(t testing.TB) context.Context {
	t.Helper()

	traceID, err := trace.TraceIDFromHex(testTraceID)
	if err != nil {
		t.Fatalf("parse trace ID: %v", err)
	}
	spanID, err := trace.SpanIDFromHex(testSpanID)
	if err != nil {
		t.Fatalf("parse span ID: %v", err)
	}

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(context.Background(), spanContext)
}

func decodeLogLine(t testing.TB, output string) map[string]any {
	t.Helper()

	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Fatalf("decode log output: %v\noutput: %s", err, output)
	}
	return entry
}

func TestAppendOTELFields(t *testing.T) {
	t.Run("valid span context", func(t *testing.T) {
		fields := appendOTELFields(contextWithTestSpan(t), nil)
		if len(fields) != 3 {
			t.Fatalf("got %d fields, want 3", len(fields))
		}
		if fields[0].Key != "trace_id" || fields[0].String != testTraceID {
			t.Errorf("unexpected trace field: %#v", fields[0])
		}
		if fields[1].Key != "span_id" || fields[1].String != testSpanID {
			t.Errorf("unexpected span field: %#v", fields[1])
		}
		if fields[2].Key != "trace_flags" || fields[2].String != "01" {
			t.Errorf("unexpected trace flags field: %#v", fields[2])
		}
	})

	t.Run("invalid and nil contexts", func(t *testing.T) {
		if fields := appendOTELFields(context.Background(), nil); len(fields) != 0 {
			t.Fatalf("invalid context added %d fields", len(fields))
		}
		if fields := appendOTELFields(nil, nil); len(fields) != 0 {
			t.Fatalf("nil context added %d fields", len(fields))
		}
	})
}

func TestInfoContext(t *testing.T) {
	var output bytes.Buffer
	logger := setupLog(
		WithOutput(&output),
		WithServiceName("payment-service"),
		WithMasking(true),
	)

	logger.InfoContext(
		contextWithTestSpan(t),
		"payment-handler",
		MESSSAGE_TYPE_EVENT,
		"payment received",
		map[string]any{"status": "received"},
	)

	entry := decodeLogLine(t, output.String())
	expected := map[string]string{
		"trace_id":         testTraceID,
		"span_id":          testSpanID,
		"trace_flags":      "01",
		"msg":              "payment received",
		"module":           "payment-handler",
		"msg_type":         "EVENT",
		"severity":         "INFO",
		"application_name": "payment-service",
	}
	for key, want := range expected {
		if got := entry[key]; got != want {
			t.Errorf("%s = %v, want %q", key, got, want)
		}
	}
}

func TestContextWithoutValidSpan(t *testing.T) {
	for _, tt := range []struct {
		name string
		ctx  context.Context
	}{
		{name: "background", ctx: context.Background()},
		{name: "nil", ctx: nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := setupLog(WithOutput(&output))
			logger.InfoContext(tt.ctx, "module", MESSSAGE_TYPE_EVENT, "still logged", nil)

			entry := decodeLogLine(t, output.String())
			if entry["msg"] != "still logged" {
				t.Errorf("msg = %v, want still logged", entry["msg"])
			}
			for _, key := range []string{"trace_id", "span_id", "trace_flags"} {
				if _, exists := entry[key]; exists {
					t.Errorf("unexpected %s field in invalid context log", key)
				}
			}
		})
	}
}

func TestContextLogLevelsAndError(t *testing.T) {
	ctx := contextWithTestSpan(t)

	t.Run("warning", func(t *testing.T) {
		var output bytes.Buffer
		logger := setupLog(WithOutput(&output))
		logger.WarningContext(ctx, "cache", "latency warning", nil)

		entry := decodeLogLine(t, output.String())
		if entry["severity"] != "WARNING" || entry["level"] != "warn" {
			t.Errorf("unexpected warning levels: %#v", entry)
		}
	})

	t.Run("error", func(t *testing.T) {
		var output bytes.Buffer
		logger := setupLog(WithOutput(&output))
		logger.ErrorContext(ctx, "database", errors.New("connection failed"))

		entry := decodeLogLine(t, output.String())
		if entry["severity"] != "ERROR" || entry["level"] != "error" {
			t.Errorf("unexpected error levels: %#v", entry)
		}
		if entry["error"] != "connection failed" {
			t.Errorf("error = %v, want connection failed", entry["error"])
		}
		if entry["trace_id"] != testTraceID || entry["span_id"] != testSpanID {
			t.Errorf("missing error correlation fields: %#v", entry)
		}
		source, ok := entry["source"].(string)
		if !ok || source == "" || strings.Contains(source, "otel_context.go") {
			t.Errorf("unexpected caller source: %v", entry["source"])
		}
	})

	t.Run("debug level", func(t *testing.T) {
		var disabledOutput bytes.Buffer
		setupLog(WithOutput(&disabledOutput)).DebugContext(ctx, "module", MESSSAGE_TYPE_EVENT, "debug", nil)
		if disabledOutput.Len() != 0 {
			t.Errorf("debug log written at default info level: %s", disabledOutput.String())
		}

		var enabledOutput bytes.Buffer
		setupLog(WithOutput(&enabledOutput), WithDebug(true)).
			DebugContext(ctx, "module", MESSSAGE_TYPE_EVENT, "debug", nil)
		entry := decodeLogLine(t, enabledOutput.String())
		if entry["severity"] != "DEBUG" || entry["level"] != "debug" {
			t.Errorf("unexpected debug levels: %#v", entry)
		}
	})
}

func TestContextMaskingAndLegacyTraceID(t *testing.T) {
	type credentials struct {
		Password string `json:"password" log:"masked:full"`
		APIKey   string `json:"api_key" log:"masked:partial"`
	}

	t.Run("context data remains masked", func(t *testing.T) {
		var output bytes.Buffer
		logger := setupLog(WithOutput(&output), WithMasking(true))
		logger.InfoContext(contextWithTestSpan(t), "auth", MESSSAGE_TYPE_EVENT, "login", credentials{
			Password: "supersecret",
			APIKey:   "sk_live_123456",
		})

		logged := output.String()
		if strings.Contains(logged, "supersecret") || strings.Contains(logged, "sk_live_123456") {
			t.Fatalf("sensitive data leaked: %s", logged)
		}
	})

	t.Run("legacy trace ID wins on legacy API", func(t *testing.T) {
		var output bytes.Buffer
		logger := setupLog(WithOutput(&output))
		logger.Info("legacy-trace-id", "module", MESSSAGE_TYPE_EVENT, "legacy", nil)

		entry := decodeLogLine(t, output.String())
		if entry["trace_id"] != "legacy-trace-id" {
			t.Errorf("trace_id = %v, want legacy-trace-id", entry["trace_id"])
		}
		if _, exists := entry["span_id"]; exists {
			t.Error("legacy API unexpectedly added span_id")
		}
	})
}

func TestInfoContextConcurrent(t *testing.T) {
	var output bytes.Buffer
	logger := setupLog(WithOutput(&output), WithServiceName("concurrent-service"))
	ctx := contextWithTestSpan(t)

	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			logger.InfoContext(ctx, "worker", MESSSAGE_TYPE_EVENT, "concurrent", nil)
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != goroutines {
		t.Fatalf("got %d log lines, want %d", len(lines), goroutines)
	}
	for _, line := range lines {
		entry := decodeLogLine(t, line)
		if entry["trace_id"] != testTraceID || entry["span_id"] != testSpanID {
			t.Errorf("correlation field leakage: %s", line)
		}
	}
}

func TestLibraryManagedTracing(t *testing.T) {
	var output bytes.Buffer
	logger := setupLog(
		WithOutput(&output),
		WithServiceName("managed-tracing-service"),
		WithTracing(),
	)
	t.Cleanup(func() {
		if err := logger.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	t.Run("log automatically creates span", func(t *testing.T) {
		output.Reset()
		logger.InfoContext(context.Background(), "automatic", MESSSAGE_TYPE_EVENT, "auto span", nil)
		entry := decodeLogLine(t, output.String())
		for _, key := range []string{"trace_id", "span_id", "trace_flags"} {
			if value, ok := entry[key].(string); !ok || value == "" {
				t.Errorf("%s missing from automatically traced log: %#v", key, entry)
			}
		}
	})

	t.Run("StartTrace groups logs", func(t *testing.T) {
		output.Reset()
		ctx, end := logger.StartTrace(context.Background(), "payment")
		logger.InfoContext(ctx, "payment", MESSSAGE_TYPE_EVENT, "first", nil)
		logger.WarningContext(ctx, "payment", "second", nil)
		end()

		lines := strings.Split(strings.TrimSpace(output.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("got %d lines, want 2", len(lines))
		}
		first := decodeLogLine(t, lines[0])
		second := decodeLogLine(t, lines[1])
		if first["trace_id"] == "" || first["trace_id"] != second["trace_id"] {
			t.Errorf("logs do not share a trace: %v != %v", first["trace_id"], second["trace_id"])
		}
	})

	t.Run("HTTP propagation", func(t *testing.T) {
		ctx, end := logger.StartTrace(context.Background(), "outgoing")
		defer end()

		header := make(http.Header)
		InjectHTTPContext(ctx, header)
		if header.Get("traceparent") == "" {
			t.Fatal("traceparent was not injected")
		}

		extracted := ExtractHTTPContext(context.Background(), header)
		output.Reset()
		logger.InfoContext(extracted, "incoming", MESSSAGE_TYPE_EVENT, "continued", nil)
		entry := decodeLogLine(t, output.String())
		originalFields := appendOTELFields(ctx, nil)
		if entry["trace_id"] != originalFields[0].String {
			t.Errorf("extracted trace_id = %v, want %s", entry["trace_id"], originalFields[0].String)
		}
	})

	if err := logger.TracingError(); err != nil {
		t.Errorf("TracingError: %v", err)
	}
}

func TestGlobalContextWrappers(t *testing.T) {
	previous := globalLog.Load()
	var output bytes.Buffer
	globalLog.Store(setupLog(WithOutput(&output), WithDebug(true)))
	t.Cleanup(func() {
		globalLog.Store(previous)
	})

	ctx := contextWithTestSpan(t)
	InfoContext(ctx, "global", MESSSAGE_TYPE_EVENT, "info", nil)
	DebugContext(ctx, "global", MESSSAGE_TYPE_EVENT, "debug", nil)
	WarningContext(ctx, "global", "warning", nil)
	ErrorContext(ctx, "global", errors.New("error"))

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d global log lines, want 4", len(lines))
	}
	for _, line := range lines {
		entry := decodeLogLine(t, line)
		if entry["trace_id"] != testTraceID || entry["span_id"] != testSpanID {
			t.Errorf("global wrapper missing correlation fields: %s", line)
		}
	}

	startedCtx, end := StartTrace(nil, "disabled-tracing")
	end()
	if startedCtx == nil {
		t.Error("StartTrace returned a nil context")
	}
	if err := TracingError(); err != nil {
		t.Errorf("TracingError = %v", err)
	}
	if err := Shutdown(nil); err != nil {
		t.Errorf("Shutdown = %v", err)
	}
}

func TestFatalContext(t *testing.T) {
	if os.Getenv("GO_WANT_FATAL_CONTEXT") == "1" {
		globalLog.Store(setupLog(WithOutput(&discardWriter{})))
		FatalContext(contextWithTestSpan(t), "fatal", errors.New("critical failure"))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestFatalContext$")
	cmd.Env = append(os.Environ(), "GO_WANT_FATAL_CONTEXT=1")
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.Success() {
		t.Fatalf("FatalContext process error = %v, want non-zero exit", err)
	}
}

func BenchmarkInfoContextWithSpan(b *testing.B) {
	logger := setupLog(WithOutput(&discardWriter{}))
	ctx := contextWithTestSpan(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.InfoContext(ctx, "api", MESSSAGE_TYPE_EVENT, "request received", nil)
	}
}

func BenchmarkInfoContextWithoutSpan(b *testing.B) {
	logger := setupLog(WithOutput(&discardWriter{}))
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.InfoContext(ctx, "api", MESSSAGE_TYPE_EVENT, "request received", nil)
	}
}

func BenchmarkErrorContextWithSpan(b *testing.B) {
	logger := setupLog(WithOutput(&discardWriter{}))
	ctx := contextWithTestSpan(b)
	err := errors.New("internal server error")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.ErrorContext(ctx, "api", err)
	}
}

func BenchmarkInfoContextAutoSpan(b *testing.B) {
	logger := setupLog(WithOutput(&discardWriter{}), WithTracing())
	b.Cleanup(func() {
		_ = logger.Shutdown(context.Background())
	})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.InfoContext(ctx, "api", MESSSAGE_TYPE_EVENT, "request received", nil)
	}
}

type discardWriter struct{}

func (*discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
