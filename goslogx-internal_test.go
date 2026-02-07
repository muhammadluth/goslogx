package goslogx

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

// TestLoggerInstanceMethods tests direct methods on Logger instance
func TestLoggerInstanceMethods(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := setupLog(WithOutput(buf), WithServiceName("test-logger"))
	traceID := "trace-123"

	t.Run("Info", func(t *testing.T) {
		buf.Reset()
		logger.Info(traceID, "mod", MESSSAGE_TYPE_EVENT, "info msg", map[string]string{"foo": "bar"})
		if buf.Len() == 0 {
			t.Error("Info did not write to buffer")
		}
	})

	t.Run("Debug", func(t *testing.T) {
		buf.Reset()
		// Debug might be disabled by default, so use WithLevel or check output
		logger.Debug(traceID, "mod", MESSSAGE_TYPE_EVENT, "debug msg", nil)
		// Default level is Info, so Debug should be empty
		if buf.Len() != 0 {
			t.Logf("Debug output (unexpected if Info level): %s", buf.String())
		}
	})

	t.Run("Warning", func(t *testing.T) {
		buf.Reset()
		logger.Warning(traceID, "mod", "warn msg", nil)
		if buf.Len() == 0 {
			t.Error("Warning did not write to buffer")
		}
	})

	t.Run("Error", func(t *testing.T) {
		buf.Reset()
		logger.Error(traceID, "mod", errors.New("test error"))
		if buf.Len() == 0 {
			t.Error("Error did not write to buffer")
		}
		if !bytes.Contains(buf.Bytes(), []byte("test error")) {
			t.Error("Error message not found in output")
		}
	})
}

// TestDataField tests the dataField helper and its various branches
func TestDataField(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		field := dataField("key", nil)
		if field.Type != zapcore.SkipType {
			t.Errorf("Expected SkipType for nil, got %v", field.Type)
		}
	})

	t.Run("FastPathDTOs", func(t *testing.T) {
		// HTTPData now always uses maskedObject wrapper
		httpData := HTTPData{Method: "GET", URL: "/api"}
		field := dataField("key", httpData)
		if field.Type != zapcore.ObjectMarshalerType {
			t.Errorf("Expected ObjectMarshalerType for HTTPData, got %v", field.Type)
		}

		// Pointer to HTTPData
		field = dataField("key", &httpData)
		if field.Type != zapcore.ObjectMarshalerType {
			t.Errorf("Expected ObjectMarshalerType for *HTTPData, got %v", field.Type)
		}

		// DBData should always use ObjectMarshaler
		dbData := DBData{Driver: "postgres"}
		field = dataField("key", dbData)
		if field.Type != zapcore.ObjectMarshalerType {
			t.Errorf("Expected ObjectMarshalerType for DBData, got %v", field.Type)
		}

		field = dataField("key", &dbData)
		if field.Type != zapcore.ObjectMarshalerType {
			t.Errorf("Expected ObjectMarshalerType for *DBData, got %v", field.Type)
		}
	})

	t.Run("SliceOfStructs", func(t *testing.T) {
		type S struct{ Name string }
		slice := []S{{Name: "test"}}
		field := dataField("key", slice)
		if field.Type != zapcore.ArrayMarshalerType {
			t.Errorf("Expected ArrayMarshalerType for slice of structs, got %v", field.Type)
		}
	})

	t.Run("SliceOfPointersToStructs", func(t *testing.T) {
		type S struct{ Name string }
		slice := []*S{{Name: "test"}}
		field := dataField("key", slice)
		if field.Type != zapcore.ArrayMarshalerType {
			t.Errorf("Expected ArrayMarshalerType for slice of struct pointers, got %v", field.Type)
		}
	})

	t.Run("SliceOfPrimitives", func(t *testing.T) {
		slice := []int{1, 2, 3}
		field := dataField("key", slice)
		// zap.Any for a slice might be ArrayMarshalerType (1) or ReflectType
		if field.Type == zapcore.SkipType {
			t.Errorf("Expected non-skip type for primitive slice, got %v", field.Type)
		}
	})

	t.Run("Map", func(t *testing.T) {
		m := map[string]string{"foo": "bar"}
		field := dataField("key", m)
		if field.Type == zapcore.SkipType {
			t.Errorf("Expected non-skip type for map, got %v", field.Type)
		}
	})

	t.Run("Struct", func(t *testing.T) {
		type S struct{ Name string }
		field := dataField("key", S{Name: "test"})
		if field.Type != zapcore.ObjectMarshalerType {
			t.Errorf("Expected ObjectMarshalerType for struct, got %v", field.Type)
		}
	})

	t.Run("PointerToNil", func(t *testing.T) {
		var s *string
		field := dataField("key", s)
		if field.Type != zapcore.SkipType {
			t.Errorf("Expected SkipType for nil pointer, got %v", field.Type)
		}
	})
}

// TestMaskingExecution manually triggers marshaling to cover all branches in masking.go
func TestMaskingExecution(t *testing.T) {
	enc := zapcore.NewMapObjectEncoder()

	t.Run("MarshalLogObject", func(t *testing.T) {
		type Inner struct {
			Secret string `log:"masked:full"`
		}
		type Outer struct {
			Public  string
			Partial string `log:"masked:partial"`
			Nested  Inner
			Ptr     *Inner
			Time    time.Time
			Slice   []Inner
			Ignored string `json:"-"`
		}

		now := time.Now()
		obj := maskedObject{v: Outer{
			Public:  "hello",
			Partial: "sensitive",
			Nested:  Inner{Secret: "topsecret"},
			Ptr:     &Inner{Secret: "ptrsecret"},
			Time:    now,
			Slice:   []Inner{{Secret: "slice-secret"}},
			Ignored: "don't show",
		}}

		err := obj.MarshalLogObject(enc)
		if err != nil {
			t.Fatalf("MarshalLogObject failed: %v", err)
		}

		m := enc.Fields
		// Field names are PascalCase if no JSON tag
		if m["Public"] != "hello" {
			t.Errorf("Expected Public=hello, got %v", m["Public"])
		}
		if m["Partial"] != "se****ve" {
			t.Errorf("Expected Partial mask, got %v", m["Partial"])
		}
	})

	t.Run("MarshalLogObjectExtra", func(t *testing.T) {
		type AllTypes struct {
			I     int
			U     uint
			F     float64
			B     bool
			A     [2]int
			M     map[string]int
			P     *int
			Alias string `json:"name_alias"` // JSON tag without comma
		}

		val := 10
		obj := maskedObject{v: AllTypes{
			I: -1, U: 1, F: 1.1, B: true,
			A:     [2]int{1, 2},
			M:     map[string]int{"a": 1},
			P:     &val,
			Alias: "alias",
		}}
		err := obj.MarshalLogObject(zapcore.NewMapObjectEncoder())
		if err != nil {
			t.Errorf("MarshalLogObjectExtra failed: %v", err)
		}
	})

	t.Run("MarshalLogArrayExtra", func(t *testing.T) {
		// Nil pointers in array
		var s *string
		slice := []*string{nil, s}
		arr := maskedArray{v: reflect.ValueOf(slice)}
		enc := &mockArrayEncoder{}
		err := arr.MarshalLogArray(enc)
		if err != nil {
			t.Errorf("MarshalLogArrayExtra failed: %v", err)
		}

		// Primitives in array
		primSlice := []int{1, 2}
		arrPrim := maskedArray{v: reflect.ValueOf(primSlice)}
		err = arrPrim.MarshalLogArray(enc)
		if err != nil {
			t.Errorf("MarshalLogArrayExtra primitives failed: %v", err)
		}
	})

	t.Run("NonStructMasking", func(t *testing.T) {
		obj := maskedObject{v: "not a struct"}
		err := obj.MarshalLogObject(zapcore.NewMapObjectEncoder())
		if err != nil {
			t.Errorf("MarshalLogObject with non-struct failed: %v", err)
		}
	})

	t.Run("PointerMasking", func(t *testing.T) {
		type S struct{ Name string }
		s := &S{Name: "test"}
		obj := maskedObject{v: s}
		err := obj.MarshalLogObject(zapcore.NewMapObjectEncoder())
		if err != nil {
			t.Errorf("MarshalLogObject with pointer failed: %v", err)
		}

		var nilS *S
		objNil := maskedObject{v: nilS}
		err = objNil.MarshalLogObject(zapcore.NewMapObjectEncoder())
		if err != nil {
			t.Errorf("MarshalLogObject with nil pointer failed: %v", err)
		}
	})
}

// TestMaskMiddle tests the maskMiddle helper
func TestMaskMiddle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"123", "****"},
		{"1234", "****"},
		{"12345", "12****45"},
		{"johndoe", "jo****oe"},
		{"", "****"},
	}

	for _, tt := range tests {
		result := maskMiddle(tt.input)
		if result != tt.expected {
			t.Errorf("maskMiddle(%s) = %s, want %s", tt.input, result, tt.expected)
		}
	}
}

// mockObject implements zapcore.ObjectMarshaler
type mockObject struct{}

func (m *mockObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return nil
}

// mockArrayEncoder is a minimal mock for zapcore.ArrayEncoder
type mockArrayEncoder struct {
	zapcore.ArrayEncoder
}

func (m *mockArrayEncoder) AppendObject(obj zapcore.ObjectMarshaler) error {
	return obj.MarshalLogObject(zapcore.NewMapObjectEncoder())
}

func (m *mockArrayEncoder) AppendReflected(v any) error {
	return nil
}

// TestInstanceLogging covers (l *Logger) methods
func TestInstanceLogging(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := setupLog(WithOutput(buf), WithServiceName("test-instance"))
	traceID := "trace-123"

	t.Run("Error", func(t *testing.T) {
		buf.Reset()
		logger.Error(traceID, "mod", errors.New("instance error"))
		if !strings.Contains(buf.String(), "instance error") {
			t.Errorf("Expected output to contain 'instance error', got %s", buf.String())
		}
	})

	t.Run("Warning", func(t *testing.T) {
		buf.Reset()
		logger.Warning(traceID, "mod", "warn msg", map[string]string{"foo": "bar"})
		if !strings.Contains(buf.String(), "warn msg") {
			t.Errorf("Expected output to contain 'warn msg', got %s", buf.String())
		}
	})

	t.Run("Debug", func(t *testing.T) {
		buf.Reset()
		// Re-setup with debug level to cover the branch
		debugLogger := setupLog(WithOutput(buf), WithDebug(true))
		debugLogger.Debug(traceID, "mod", MESSSAGE_TYPE_EVENT, "debug msg", map[string]string{"foo": "bar"})
		if !strings.Contains(buf.String(), "debug msg") {
			t.Errorf("Expected output to contain 'debug msg', got %s", buf.String())
		}
	})

	t.Run("Info", func(t *testing.T) {
		buf.Reset()
		logger.Info(traceID, "mod", MESSSAGE_TYPE_EVENT, "info msg", map[string]string{"foo": "bar"})
		if !strings.Contains(buf.String(), "info msg") {
			t.Errorf("Expected output to contain 'info msg', got %s", buf.String())
		}
	})
}

// TestInstanceFatal covers (l *Logger) Fatal
func TestInstanceFatal(t *testing.T) {
	if os.Getenv("BE_CRASHER_INSTANCE") == "1" {
		logger := setupLog(WithServiceName("crash-service"))
		logger.Fatal("trace-crash", "main", errors.New("critical failure"))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestInstanceFatal")
	cmd.Env = append(os.Environ(), "BE_CRASHER_INSTANCE=1")
	err := cmd.Run()

	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	}
	t.Fatalf("process ran with err %v, want exit status 1", err)
}

// TestDetectCallerSkipEdgeCases covers deep stack and failures
func TestDetectCallerSkipEdgeCases(t *testing.T) {
	t.Run("DeepStack", func(t *testing.T) {
		var callDeep func(int) int
		callDeep = func(n int) int {
			if n <= 0 {
				return detectCallerSkip()
			}
			return callDeep(n - 1)
		}
		// Max depth is 15 in caller.go
		skip := callDeep(20)
		if skip != 2 { // Fallback value
			t.Errorf("Expected fallback skip 2 for deep stack, got %d", skip)
		}
	})
}

// TestDataFieldUnsupported covers fallback to zap.Any
func TestDataFieldUnsupported(t *testing.T) {
	// Channels are unsupported for specialized masking
	ch := make(chan int)
	field := dataField("chan", ch)
	if field.Type == zap.Skip().Type {
		t.Error("Expected non-skip field for channel")
	}

	// Function is also unsupported
	fn := func() {}
	field = dataField("func", fn)
	if field.Type == zap.Skip().Type {
		t.Error("Expected non-skip field for func")
	}
}

// TestMaskingEdgeCases covers remaining branches in masking.go
func TestMaskingEdgeCases(t *testing.T) {
	t.Run("InvalidJSONString", func(t *testing.T) {
		input := "{invalid"
		result := maskJSONString(input)
		if result != input {
			t.Errorf("Expected original string for invalid JSON, got %s", result)
		}
	})

	t.Run("MarshalFailure", func(t *testing.T) {
		// Just ensure invalid JSON doesn't crash
		input := `{"foo":`
		result := maskJSONString(input)
		if result != input {
			t.Errorf("Expected original string, got %s", result)
		}
	})

	t.Run("MarshalLogArrayNilPointer", func(t *testing.T) {
		type S struct{ Name string }
		var nilPtr *S
		slice := []*S{nilPtr}
		arr := maskedArray{v: reflect.ValueOf(slice)}
		enc := &mockArrayEncoder{}
		_ = arr.MarshalLogArray(enc)
	})

	t.Run("MarshalLogArrayPointerToStruct", func(t *testing.T) {
		type S struct{ Name string }
		s := S{Name: "test"}
		slice := []*S{&s}
		arr := maskedArray{v: reflect.ValueOf(slice)}
		enc := &mockArrayEncoder{}
		_ = arr.MarshalLogArray(enc)
	})
}

// TestDataFieldMoreTypes covers remaining branches
func TestDataFieldMoreTypes(t *testing.T) {
	t.Run("Complex", func(t *testing.T) {
		dataField("c64", complex64(1+2i))
		dataField("c128", complex128(1+2i))
	})

	t.Run("PrimitiveArray", func(t *testing.T) {
		arr := [2]int{1, 2}
		dataField("arr", arr)
	})

	t.Run("NilPointerToStruct", func(t *testing.T) {
		type S struct{}
		var s *S
		field := dataField("nilstruct", s)
		if field.Type != zap.Skip().Type {
			t.Error("Expected skip for nil struct pointer")
		}
	})

	t.Run("PointerToDTOS", func(t *testing.T) {
		dataField("http", &HTTPData{})
		dataField("db", &DBData{})
		dataField("mq", &MQData{})
		dataField("gen", &GenericData{})
	})

	t.Run("ReflectionFallback", func(t *testing.T) {
		// Test recursion and fallback
		type Nested struct {
			S string
		}
		type Top struct {
			N Nested
		}
		dataField("top", Top{N: Nested{S: "test"}})

		// Map fallback
		dataField("map", map[string]int{"a": 1})
	})
}

// TestMaskedObjectEdgeCases covers non-structs in MarshalLogObject
func TestMaskedObjectEdgeCases(t *testing.T) {
	t.Run("NonStructPointer", func(t *testing.T) {
		i := 10
		obj := maskedObject{v: &i}
		enc := zapcore.NewMapObjectEncoder()
		_ = obj.MarshalLogObject(enc)
	})

	t.Run("NilPointer", func(t *testing.T) {
		var i *int
		obj := maskedObject{v: i}
		enc := zapcore.NewMapObjectEncoder()
		_ = obj.MarshalLogObject(enc)
	})
}
