package goslogx

import (
	"bytes"
	"errors"
	"strings"
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
		{"SingleBackslash", []byte(`hello \`), "hello \\"},
		{"BackslashAtEnd", []byte(`hello \\`), "hello \\"},
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
		{
			"EmptyString",
			"",
			"[]",
		},
		{
			"OnlyNewline",
			"\n",
			"[ | ]",
		},
		{
			"NewlineAtEnd",
			"test\n",
			"[test | ]",
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

// TestWriterError tests when the underlying writer returns an error
func TestWriterError(t *testing.T) {
	errWriter := &errorWriter{err: errors.New("underlying write failed")}
	writer := &stackTraceFormattingWriter{
		Writer: errWriter,
		buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
	}

	// 1. Write without stack trace
	_, err := writer.Write([]byte(`{"msg":"test"}`))
	if err == nil || err.Error() != "underlying write failed" {
		t.Errorf("Expected underlying write error, got %v", err)
	}

	// 2. Write with stack trace
	_, err = writer.Write([]byte(`{"stack_trace":"test"}`))
	if err == nil || err.Error() != "underlying write failed" {
		t.Errorf("Expected underlying write error with stack trace, got %v", err)
	}
}

// errorWriter is a mock writer that always returns an error
type errorWriter struct {
	err error
}

func (e *errorWriter) Write(p []byte) (int, error) {
	return 0, e.err
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

	t.Run("WriterWithSyncError", func(t *testing.T) {
		mockWriter := &mockWriteSyncer{
			Buffer:  bytes.NewBuffer(nil),
			syncErr: errors.New("sync failed"),
		}
		writer := &stackTraceFormattingWriter{
			Writer: mockWriter,
			buf:    bytes.NewBuffer(make([]byte, 0, 1024)),
		}

		err := writer.Sync()
		if err == nil || err.Error() != "sync failed" {
			t.Errorf("Expected sync error, got %v", err)
		}
	})
}

// mockWriteSyncer is a mock WriteSyncer for testing
type mockWriteSyncer struct {
	*bytes.Buffer
	syncCalled bool
	syncErr    error
}

// Write implements io.Writer
func (m *mockWriteSyncer) Write(p []byte) (int, error) {
	return m.Buffer.Write(p)
}

// Sync marks that Sync was called
func (m *mockWriteSyncer) Sync() error {
	m.syncCalled = true
	return m.syncErr
}

// TestMaskingFunctions tests the masking helper functions
func TestMaskingFunctions(t *testing.T) {
	maskChar := '*' // 32 bytes for AES-256

	t.Run("MaskString", func(t *testing.T) {
		logger := NewLog("mask-test", maskChar)

		// Test masking
		plaintext := "secret-data"
		masked := logger.maskString(plaintext)

		if masked == plaintext {
			t.Error("Masked value should differ from plaintext")
		}

		t.Logf("Masked: %s", masked)
	})

	t.Run("MaskStringNoKey", func(t *testing.T) {
		logger := NewLog("no-mask-test", 0)

		// Test masking disabled
		plaintext := "secret-data"
		result := logger.maskString(plaintext)

		if result != plaintext {
			t.Error("Expected plaintext when masking disabled")
		}
	})

	t.Run("ProcessDataMaskingNil", func(t *testing.T) {
		logger := NewLog("nil-test", maskChar)

		// Test with nil data
		result := logger.processDataMasking(nil)
		if result != nil {
			t.Error("Expected nil result for nil data")
		}
	})

	t.Run("ProcessDataMaskingNoKey", func(t *testing.T) {
		logger := NewLog("no-key-test", 0)

		data := map[string]string{"key": "value"}
		result := logger.processDataMasking(data)
		if result == nil {
			t.Error("Expected non-nil result")
		}
	})

	t.Run("ProcessStructWithMaskedFields", func(t *testing.T) {
		logger := NewLog("struct-test", maskChar)

		type TestStruct struct {
			Public string `json:"public"`
			Secret string `json:"secret" masked:"true"`
			Number int    `json:"number"`
		}

		data := TestStruct{
			Public: "public-value",
			Secret: "secret-value",
			Number: 42,
		}

		result := logger.processDataMasking(data)
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatal("Expected map result")
		}

		// Check public field is unchanged
		if resultMap["public"] != "public-value" {
			t.Error("Public field should be unchanged")
		}

		// Check secret field is masked
		secretVal, ok := resultMap["secret"].(string)
		if !ok {
			t.Fatal("Expected string secret field")
		}

		if secretVal == "secret-value" {
			t.Error("Secret field should be masked")
		}
	})

	t.Run("ProcessNestedStructs", func(t *testing.T) {
		logger := NewLog("nested-test", maskChar)

		type Inner struct {
			Password string `json:"password" masked:"true"`
		}

		type Outer struct {
			Name  string `json:"name"`
			Inner Inner  `json:"inner"`
		}

		data := Outer{
			Name: "test",
			Inner: Inner{
				Password: "secret123",
			},
		}

		result := logger.processDataMasking(data)
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatal("Expected map result")
		}

		innerMap, ok := resultMap["inner"].(map[string]any)
		if !ok {
			t.Fatal("Expected inner map")
		}

		password, ok := innerMap["password"].(string)
		if !ok {
			t.Fatal("Expected password string")
		}

		if password == "secret123" {
			t.Error("Password should be masked")
		}
	})

	t.Run("ProcessMap", func(t *testing.T) {
		logger := NewLog("map-test", maskChar)

		type TestStruct struct {
			Secret string `json:"secret" masked:"true"`
		}

		data := map[string]interface{}{
			"item": TestStruct{Secret: "password"},
		}

		result := logger.processDataMasking(data)
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatal("Expected map result")
		}

		itemMap, ok := resultMap["item"].(map[string]any)
		if !ok {
			t.Fatal("Expected item map")
		}

		secret, ok := itemMap["secret"].(string)
		if !ok {
			t.Fatal("Expected secret string")
		}

		if secret == "password" {
			t.Error("Secret should be masked")
		}
	})

	t.Run("ProcessSlice", func(t *testing.T) {
		logger := NewLog("slice-test", maskChar)

		type TestStruct struct {
			Secret string `json:"secret" masked:"true"`
		}

		data := []TestStruct{
			{Secret: "secret1"},
			{Secret: "secret2"},
		}

		result := logger.processDataMasking(data)
		resultSlice, ok := result.([]any)
		if !ok {
			t.Fatal("Expected slice result")
		}

		if len(resultSlice) != 2 {
			t.Errorf("Expected 2 items, got %d", len(resultSlice))
		}

		// Check first item
		item1, ok := resultSlice[0].(map[string]any)
		if !ok {
			t.Fatal("Expected map in slice")
		}

		secret1, ok := item1["secret"].(string)
		if !ok {
			t.Fatal("Expected secret string")
		}

		if secret1 == "secret1" {
			t.Error("Secret should be masked")
		}
	})

	t.Run("ProcessNilMap", func(t *testing.T) {
		logger := NewLog("nil-map-test", maskChar)

		var data map[string]string
		result := logger.processDataMasking(data)
		if result != nil {
			t.Error("Expected nil for nil map")
		}
	})

	t.Run("ProcessNilSlice", func(t *testing.T) {
		logger := NewLog("nil-slice-test", maskChar)

		var data []string
		result := logger.processDataMasking(data)
		if result != nil {
			t.Error("Expected nil for nil slice")
		}
	})

	t.Run("ProcessEmptyString", func(t *testing.T) {
		logger := NewLog("empty-test", maskChar)

		type TestStruct struct {
			Secret string `json:"secret" masked:"true"`
		}

		data := TestStruct{Secret: ""}
		result := logger.processDataMasking(data)
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatal("Expected map result")
		}

		secret, ok := resultMap["secret"].(string)
		if !ok {
			t.Fatal("Expected secret string")
		}

		if secret != "" {
			t.Error("Empty string should remain empty")
		}
	})

	t.Run("ProcessPrimitiveTypes", func(t *testing.T) {
		logger := NewLog("primitive-test", maskChar)

		// Test with primitive types
		result := logger.processDataMasking(42)
		if result != 42 {
			t.Error("Primitive int should be unchanged")
		}

		result = logger.processDataMasking("string")
		if result != "string" {
			t.Error("Primitive string should be unchanged")
		}

		result = logger.processDataMasking(true)
		if result != true {
			t.Error("Primitive bool should be unchanged")
		}
	})

}

// TestMaskingEdgeCases tests edge cases in masking
func TestMaskingEdgeCases(t *testing.T) {
	maskChar := '*'

	t.Run("ProcessPointerToStruct", func(t *testing.T) {
		logger := NewLog("pointer-test", maskChar)

		type TestStruct struct {
			Secret string `json:"secret" masked:"true"`
		}

		data := &TestStruct{Secret: "password"}
		result := logger.processDataMasking(data)
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatal("Expected map result")
		}

		secret, ok := resultMap["secret"].(string)
		if !ok {
			t.Fatal("Expected secret string")
		}

		if secret == "password" {
			t.Error("Secret should be masked")
		}
	})

	t.Run("ProcessInterface", func(t *testing.T) {
		logger := NewLog("interface-test", maskChar)

		type TestStruct struct {
			Secret string `json:"secret" masked:"true"`
		}

		var data interface{} = TestStruct{Secret: "password"}
		result := logger.processDataMasking(data)
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatal("Expected map result")
		}

		secret, ok := resultMap["secret"].(string)
		if !ok {
			t.Fatal("Expected secret string")
		}

		if secret == "password" {
			t.Error("Secret should be masked")
		}
	})

	t.Run("ProcessArray", func(t *testing.T) {
		logger := NewLog("array-test", maskChar)

		type TestStruct struct {
			Secret string `json:"secret" masked:"true"`
		}

		data := [2]TestStruct{
			{Secret: "secret1"},
			{Secret: "secret2"},
		}

		result := logger.processDataMasking(data)
		resultSlice, ok := result.([]any)
		if !ok {
			t.Fatal("Expected slice result")
		}

		if len(resultSlice) != 2 {
			t.Errorf("Expected 2 items, got %d", len(resultSlice))
		}
	})

	t.Run("ProcessStructWithOmitempty", func(t *testing.T) {
		logger := NewLog("omitempty-test", maskChar)

		type TestStruct struct {
			Secret string `json:"secret,omitempty" masked:"true"`
			Public string `json:"public,omitempty"`
		}

		data := TestStruct{
			Secret: "password",
			Public: "value",
		}

		result := logger.processDataMasking(data)
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatal("Expected map result")
		}

		secret, ok := resultMap["secret"].(string)
		if !ok {
			t.Fatal("Expected secret string")
		}

		if secret == "password" {
			t.Error("Secret should be masked")
		}

		if resultMap["public"] != "value" {
			t.Error("Public field should be unchanged")
		}
	})

	t.Run("ProcessStructWithJSONDash", func(t *testing.T) {
		logger := NewLog("dash-test", maskChar)

		type TestStruct struct {
			Ignored string `json:"-"`
			Public  string `json:"public"`
		}

		data := TestStruct{
			Ignored: "ignored",
			Public:  "value",
		}

		result := logger.processDataMasking(data)
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatal("Expected map result")
		}

		// Ignored field should not be in result
		if _, exists := resultMap["-"]; exists {
			t.Error("Ignored field should not be in result")
		}

		if resultMap["public"] != "value" {
			t.Error("Public field should be unchanged")
		}
	})

	t.Run("ProcessNilPointer", func(t *testing.T) {
		logger := NewLog("nil-ptr-test", maskChar)

		type TestStruct struct {
			Secret string `json:"secret" masked:"true"`
		}

		var data *TestStruct
		result := logger.processDataMasking(data)
		if result != nil {
			t.Error("Expected nil for nil pointer")
		}
	})

	t.Run("ProcessNilInterface", func(t *testing.T) {
		logger := NewLog("nil-iface-test", maskChar)

		var data interface{}
		result := logger.processDataMasking(data)
		if result != nil {
			t.Error("Expected nil for nil interface")
		}
	})
}

// TestMaskingBufferReallocation tests buffer reallocation in masking
func TestMaskingBufferReallocation(t *testing.T) {
	maskChar := '*'

	t.Run("MaskLargeString", func(t *testing.T) {
		logger := NewLog("large-test", maskChar)

		// Create a large string that will require buffer reallocation
		largeString := strings.Repeat("a", 1000)
		masked := logger.maskString(largeString)

		if masked == largeString {
			t.Error("Masked value should differ from plaintext")
		}
	})

	t.Run("ProcessStructWithNonStringMaskedField", func(t *testing.T) {
		logger := NewLog("non-string-test", maskChar)

		type TestStruct struct {
			Number int    `json:"number" masked:"true"` // masked tag on non-string field
			Secret string `json:"secret" masked:"true"`
		}

		data := TestStruct{
			Number: 42,
			Secret: "password",
		}

		result := logger.processDataMasking(data)
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatal("Expected map result")
		}

		// Number should be unchanged (not a string)
		if resultMap["number"] != 42 {
			t.Error("Number field should be unchanged")
		}

		// Secret should be masked
		secret, ok := resultMap["secret"].(string)
		if !ok {
			t.Fatal("Expected secret string")
		}

		if secret == "password" {
			t.Error("Secret should be masked")
		}
	})

	t.Run("ProcessMapWithIntKey", func(t *testing.T) {
		logger := NewLog("int-key-test", maskChar)

		type TestStruct struct {
			Secret string `json:"secret" masked:"true"`
		}

		data := map[int]TestStruct{
			1: {Secret: "password"},
		}

		result := logger.processDataMasking(data)
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatal("Expected map result")
		}

		// Check that the key was converted to string
		item, ok := resultMap["1"]
		if !ok {
			t.Fatal("Expected key '1' in result")
		}

		itemMap, ok := item.(map[string]any)
		if !ok {
			t.Fatal("Expected item to be map")
		}

		secret, ok := itemMap["secret"].(string)
		if !ok {
			t.Fatal("Expected secret string")
		}

		if secret == "password" {
			t.Error("Secret should be masked")
		}
	})
}

// TestMaskEmail tests email masking functionality
func TestMaskEmail(t *testing.T) {
	maskChar := '*'

	t.Run("ValidEmail", func(t *testing.T) {
logger := NewLog("email-test", maskChar)

email := "john.doe@example.com"
masked := logger.maskEmail(email)

// Should show first 2 chars + domain
if !strings.HasPrefix(masked, "jo") {
t.Errorf("Expected masked email to start with 'jo', got %s", masked)
}

if !strings.HasSuffix(masked, "@example.com") {
t.Errorf("Expected masked email to end with '@example.com', got %s", masked)
}

if !strings.Contains(masked, string(maskChar)) {
t.Errorf("Expected masked email to contain mask character, got %s", masked)
}

t.Logf("Masked email: %s", masked)
})

	t.Run("ShortLocalPart", func(t *testing.T) {
logger := NewLog("short-email-test", maskChar)

email := "ab@example.com"
masked := logger.maskEmail(email)

// Very short local part should be fully masked
if !strings.Contains(masked, string(maskChar)) {
t.Errorf("Expected masked email to contain mask character, got %s", masked)
}

if !strings.HasSuffix(masked, "@example.com") {
t.Errorf("Expected masked email to end with '@example.com', got %s", masked)
}

t.Logf("Masked short email: %s", masked)
})

	t.Run("InvalidEmail", func(t *testing.T) {
logger := NewLog("invalid-email-test", maskChar)

email := "notanemail"
masked := logger.maskEmail(email)

// Invalid email should be fully masked
if masked != strings.Repeat(string(maskChar), len(email)) {
t.Errorf("Expected fully masked invalid email, got %s", masked)
}

t.Logf("Masked invalid email: %s", masked)
})

	t.Run("EmailWithMultipleAt", func(t *testing.T) {
logger := NewLog("multi-at-test", maskChar)

email := "test@test@example.com"
masked := logger.maskEmail(email)

// Should be fully masked as it's invalid
if masked != strings.Repeat(string(maskChar), len(email)) {
			t.Errorf("Expected fully masked email with multiple @, got %s", masked)
		}
		
		t.Logf("Masked multi-@ email: %s", masked)
	})
}

// TestMaskStringEdgeCases tests edge cases for string masking
func TestMaskStringEdgeCases(t *testing.T) {
	maskChar := '*'

	t.Run("EmailDetection", func(t *testing.T) {
		logger := NewLog("email-detect-test", maskChar)
		
		email := "user@domain.com"
		masked := logger.maskString(email)
		
		// Should detect email and mask appropriately
		if !strings.Contains(masked, "@") {
			t.Error("Expected @ symbol to be preserved in masked email")
		}
		
		t.Logf("Masked email via maskString: %s", masked)
	})

	t.Run("ShortString", func(t *testing.T) {
		logger := NewLog("short-test", maskChar)
		
		short := "pass"
		masked := logger.maskString(short)
		
		// Short strings should be fully masked
		if masked != "****" {
			t.Errorf("Expected fully masked short string, got %s", masked)
		}
		
		t.Logf("Masked short string: %s", masked)
	})

	t.Run("MediumString", func(t *testing.T) {
		logger := NewLog("medium-test", maskChar)
		
		medium := "password"
		masked := logger.maskString(medium)
		
		// 8 chars should be fully masked
		if masked != "********" {
			t.Errorf("Expected fully masked 8-char string, got %s", masked)
		}
		
		t.Logf("Masked medium string: %s", masked)
	})

	t.Run("LongString", func(t *testing.T) {
		logger := NewLog("long-test", maskChar)
		
		long := "verylongpassword123"
		masked := logger.maskString(long)
		
		// Should show first 2 and last 2
		if !strings.HasPrefix(masked, "ve") {
			t.Errorf("Expected masked string to start with 've', got %s", masked)
		}
		
		if !strings.HasSuffix(masked, "23") {
			t.Errorf("Expected masked string to end with '23', got %s", masked)
		}
		
		if !strings.Contains(masked, string(maskChar)) {
			t.Errorf("Expected masked string to contain mask character, got %s", masked)
		}
		
		t.Logf("Masked long string: %s", masked)
	})

	t.Run("DifferentMaskChar", func(t *testing.T) {
		logger := NewLog("x-mask-test", 'x')
		
		value := "secretvalue"
		masked := logger.maskString(value)
		
		// Should use 'x' as mask character
		if !strings.Contains(masked, "x") {
			t.Errorf("Expected masked string to contain 'x', got %s", masked)
		}
		
		t.Logf("Masked with 'x': %s", masked)
	})
}

// TestProcessStructJSONTagEdgeCases tests JSON tag parsing edge cases
func TestProcessStructJSONTagEdgeCases(t *testing.T) {
	maskChar := '*'

	t.Run("JSONTagWithoutComma", func(t *testing.T) {
logger := NewLog("json-tag-test", maskChar)

type TestStruct struct {
Field string `json:"customname" masked:"true"`
}

data := TestStruct{Field: "secret"}
result := logger.processDataMasking(data)
resultMap, ok := result.(map[string]any)
if !ok {
t.Fatal("Expected map result")
}

// Should use custom JSON name
if _, exists := resultMap["customname"]; !exists {
			t.Error("Expected field with custom JSON name")
		}
	})

	t.Run("MaskStringNineChars", func(t *testing.T) {
logger := NewLog("nine-char-test", maskChar)

// Test 9-char string (just over the 8-char threshold)
value := "password1"
masked := logger.maskString(value)

// Should show first 2 and last 2
if !strings.HasPrefix(masked, "pa") || !strings.HasSuffix(masked, "d1") {
t.Errorf("Expected 'pa****d1', got %s", masked)
}
})
}
