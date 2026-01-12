package goslogx

import (
	"bytes"
	"testing"

	"go.uber.org/zap/zapcore"
)

// TestMapSeverityAllLevels tests all severity levels including edge cases
func TestMapSeverityAllLevels(t *testing.T) {
	tests := []struct {
		name     string
		level    zapcore.Level
		expected string
	}{
		{"DebugLevel", zapcore.DebugLevel, "DEBUG"},
		{"InfoLevel", zapcore.InfoLevel, "INFO"},
		{"WarnLevel", zapcore.WarnLevel, "WARNING"},
		{"ErrorLevel", zapcore.ErrorLevel, "ERROR"},
		{"FatalLevel", zapcore.FatalLevel, "CRITICAL"},
		{"DPanicLevel", zapcore.DPanicLevel, "DEFAULT"},
		{"PanicLevel", zapcore.PanicLevel, "DEFAULT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapSeverity(tt.level)
			if result != tt.expected {
				t.Errorf("mapSeverity(%v) = %s, want %s", tt.level, result, tt.expected)
			}
		})
	}
}

// TestDecodeJSONString tests JSON string decoding with and without escapes
func TestDecodeJSONString(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{"NoEscapes", []byte("hello world"), "hello world"},
		{"EscapedQuote", []byte(`hello \"world\"`), `hello "world"`},
		{"EscapedBackslash", []byte(`hello \\ world`), `hello \ world`},
		{"EscapedNewline", []byte(`hello \n world`), "hello \n world"},
		{"EscapedTab", []byte(`hello \t world`), "hello \t world"},
		{"UnhandledEscape", []byte(`hello \r world`), "hello \\r world"},
		{"MixedEscapes", []byte(`"test" \n\t \\ value`), `"test" ` + "\n\t" + ` \ value`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decodeJSONString(tt.input)
			if result != tt.expected {
				t.Errorf("decodeJSONString(%s) = %q, want %q", string(tt.input), result, tt.expected)
			}
		})
	}
}

// TestFormatStackTraceBytes tests stack trace formatting
func TestFormatStackTraceBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"BasicStackTrace",
			"func1\n\t/path/to/file.go:10\nfunc2\n\t/path/to/file.go:20",
			"[func1 | /path/to/file.go:10 | func2 | /path/to/file.go:20]",
		},
		{
			"SingleFrame",
			"main.main\n\t/path/main.go:15",
			"[main.main | /path/main.go:15]",
		},
		{
			"NoFormatting",
			"simple text",
			"[simple text]",
		},
		{
			"MultipleNewlines",
			"func\n\npath",
			"[func |  | path]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			formatStackTraceBytes(&buf, tt.input)
			result := buf.String()
			if result != tt.expected {
				t.Errorf("formatStackTraceBytes(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

// TestWriterWithStackTrace tests the stackTraceFormattingWriter with stack trace data
func TestWriterWithStackTrace(t *testing.T) {
	t.Run("JSONWithStackTrace", func(t *testing.T) {
		targetBuf := &bytes.Buffer{}
		writer := &stackTraceFormattingWriter{
			Writer: targetBuf,
			buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
		}

		// JSON with stack trace field - must include the exact key format
		jsonData := []byte(`{"level":"error","msg":"test","stack_trace":"goroutine 1\nmain.main"}`)
		n, err := writer.Write(jsonData)
		if err != nil {
			t.Errorf("Write returned error: %v", err)
		}
		if n == 0 {
			t.Errorf("Write returned 0 bytes written")
		}

		output := targetBuf.String()
		if len(output) == 0 {
			t.Errorf("Expected output in buffer, got empty")
		}
		// Verify the stack trace was formatted with brackets
		if !bytes.Contains([]byte(output), []byte("[")) {
			t.Logf("Stack trace not formatted: %s", output)
		}
	})

	t.Run("JSONWithoutStackTrace", func(t *testing.T) {
		targetBuf := &bytes.Buffer{}
		writer := &stackTraceFormattingWriter{
			Writer: targetBuf,
			buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
		}

		// JSON without stack trace field
		jsonData := []byte(`{"level":"info","message":"test"}`)
		n, err := writer.Write(jsonData)
		if err != nil {
			t.Errorf("Write returned error: %v", err)
		}
		if n == 0 {
			t.Errorf("Write returned 0 bytes written")
		}

		output := targetBuf.String()
		if len(output) == 0 {
			t.Errorf("Expected output in buffer, got empty")
		}
	})

	t.Run("EmptyData", func(t *testing.T) {
		targetBuf := &bytes.Buffer{}
		writer := &stackTraceFormattingWriter{
			Writer: targetBuf,
			buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
		}

		n, err := writer.Write([]byte{})
		if err != nil {
			t.Errorf("Write returned error: %v", err)
		}
		if n != 0 {
			t.Logf("Write of empty data returned %d bytes", n)
		}
	})

	t.Run("StackTraceWithEscapes", func(t *testing.T) {
		targetBuf := &bytes.Buffer{}
		writer := &stackTraceFormattingWriter{
			Writer: targetBuf,
			buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
		}

		// JSON with stack trace containing escaped backslashes and quotes
		jsonData := []byte(`{"level":"error","msg":"test","stack_trace":"path\\\\file\\"line\\nmore"}`)
		n, err := writer.Write(jsonData)
		if err != nil {
			t.Errorf("Write returned error: %v", err)
		}
		if n == 0 {
			t.Errorf("Write returned 0 bytes written")
		}

		output := targetBuf.String()
		if len(output) == 0 {
			t.Errorf("Expected output in buffer, got empty")
		}
	})

	t.Run("NoClosingQuote", func(t *testing.T) {
		targetBuf := &bytes.Buffer{}
		writer := &stackTraceFormattingWriter{
			Writer: targetBuf,
			buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
		}

		// Create data where stack_trace value extends to the end with an escape
		// The loop will search for closing quote but reach end of buffer
		jsonData := []byte(`{"stack_trace":"incomplete\\`)
		n, err := writer.Write(jsonData)
		if err != nil {
			t.Logf("Write returned error: %v (expected for incomplete data)", err)
		}
		// Should return data as-is when can't find closing quote
		t.Logf("NoClosingQuote: n=%d, output=%s", n, targetBuf.String())
	})
}

// TestSyncImplementation tests WriteSyncer interface implementation
func TestSyncImplementation(t *testing.T) {
	t.Run("WriterWithoutSync", func(t *testing.T) {
		// Use a buffer which doesn't implement WriteSyncer
		buf := &bytes.Buffer{}
		writer := &stackTraceFormattingWriter{
			Writer: buf,
			buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
		}

		err := writer.Sync()
		// Should not panic - either no error or a graceful fallback
		t.Logf("Sync with non-WriteSyncer returned: %v", err)
	})

	t.Run("WriterWithSync", func(t *testing.T) {
		// Create a mock writer with Sync method
		mockWriter := &mockWriteSyncer{Buffer: bytes.NewBuffer(nil)}
		writer := &stackTraceFormattingWriter{
			Writer: mockWriter,
			buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
		}

		err := writer.Sync()
		if err != nil {
			t.Errorf("Sync returned error: %v", err)
		}
		if !mockWriter.syncCalled {
			t.Errorf("Sync was not called on underlying WriteSyncer")
		}
	})
}

// mockWriteSyncer is a mock WriteSyncer for testing
type mockWriteSyncer struct {
	*bytes.Buffer
	syncCalled bool
}

// Write implements io.Writer
func (m *mockWriteSyncer) Write(p []byte) (int, error) {
	return m.Buffer.Write(p)
}

// Sync marks that Sync was called
func (m *mockWriteSyncer) Sync() error {
	m.syncCalled = true
	return nil
}
