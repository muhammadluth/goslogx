package goslogx

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
)

type coverageJSONValue struct {
	Password string
}

func (v coverageJSONValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"password": v.Password})
}

type coverageJSONPointer struct {
	Email string
}

func (v *coverageJSONPointer) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"email": v.Email})
}

func TestAutoMaskingValueKinds(t *testing.T) {
	number := 42
	payload := map[string]any{
		"password":  "secret",
		"email":     "user@example.com",
		"plain":     "visible",
		"bool":      true,
		"int":       int32(-12),
		"uint":      uint64(12),
		"float":     12.5,
		"time":      time.Unix(1, 0).UTC(),
		"pointer":   &number,
		"nil_map":   map[string]string(nil),
		"nil_slice": []string(nil),
		"nested":    map[string]any{"token": "nested-secret"},
		"array": []any{
			nil,
			(*int)(nil),
			&number,
			coverageJSONValue{Password: "custom-secret"},
			&coverageJSONPointer{Email: "custom@example.com"},
			time.Unix(2, 0).UTC(),
			map[string]any{"authorization": "nested-auth"},
			[]any{true, int8(-1), uint16(2), float32(3.5)},
			map[int]string{1: "one"},
		},
	}

	var output bytes.Buffer
	logger := setupLog(WithOutput(&output))
	logger.Info("trace", "coverage", MESSSAGE_TYPE_EVENT, "all kinds", GenericData{
		Service: "coverage",
		Action:  "encode",
		Payload: payload,
	})

	result := output.String()
	for _, secret := range []string{"secret\"", "user@example.com", "custom@example.com", "nested-auth"} {
		if strings.Contains(result, secret) {
			t.Fatalf("sensitive value %q was not masked: %s", secret, result)
		}
	}
	if !json.Valid(bytes.TrimSpace(output.Bytes())) {
		t.Fatalf("logger produced invalid JSON: %s", result)
	}
}

func TestAutoStructTagsAndKinds(t *testing.T) {
	type nested struct {
		Token string `json:"token"`
	}
	type payload struct {
		Skip       string              `json:"-"`
		Empty      string              `json:"empty,omitempty"`
		Password   string              `json:"password"`
		Explicit   string              `json:"alias" log:"masked:partial"`
		Nested     nested              `json:"nested"`
		NestedPtr  *nested             `json:"nested_ptr"`
		Items      []nested            `json:"items"`
		Pointers   []*nested           `json:"pointers"`
		Headers    map[string][]string `json:"headers" log:"masked:headers"`
		Auto       any                 `json:"auto" log:"masked:auto"`
		Integer    int                 `json:"integer"`
		Unsigned   uint                `json:"unsigned"`
		Float      float64             `json:"float"`
		Enabled    bool                `json:"enabled"`
		Complex    complex64           `json:"complex"`
		ByteValues []byte              `json:"bytes"`
	}

	value := payload{
		Skip:      "must-not-appear",
		Password:  "secret",
		Explicit:  "abcdef",
		Nested:    nested{Token: "nested"},
		NestedPtr: &nested{Token: "pointer"},
		Items:     []nested{{Token: "item"}},
		Pointers:  []*nested{{Token: "pointer-item"}, nil},
		Headers: map[string][]string{
			"Authorization": {"Bearer secret"},
			"X-API-Key":     {"abcdef"},
		},
		Auto:       `{"email":"auto@example.com"}`,
		Integer:    -1,
		Unsigned:   2,
		Float:      3.5,
		Enabled:    true,
		ByteValues: []byte{1, 2},
	}

	enc := zapcore.NewMapObjectEncoder()
	if err := (maskedObject{v: value, mode: maskAuto}).MarshalLogObject(enc); err == nil {
		t.Fatal("expected unsupported complex value to return an encoding error")
	}
	value.Complex = 0
	enc = zapcore.NewMapObjectEncoder()
	if err := (maskedObject{v: value, mode: maskAuto}).MarshalLogObject(enc); err == nil {
		// complex numbers, including zero, are not JSON encodable
		t.Fatal("expected complex value to remain unsupported")
	}

	type supportedPayload struct {
		Skip      string              `json:"-"`
		Empty     string              `json:"empty,omitempty"`
		Password  string              `json:"password"`
		Explicit  string              `json:"alias" log:"masked:partial"`
		Nested    nested              `json:"nested"`
		Items     []nested            `json:"items"`
		Headers   map[string][]string `json:"headers" log:"masked:headers"`
		Auto      any                 `json:"auto" log:"masked:auto"`
		Integer   int                 `json:"integer"`
		Unsigned  uint                `json:"unsigned"`
		Float     float64             `json:"float"`
		Enabled   bool                `json:"enabled"`
		ByteValue []byte              `json:"bytes"`
	}
	enc = zapcore.NewMapObjectEncoder()
	if err := (maskedObject{v: supportedPayload{
		Skip:      "must-not-appear",
		Password:  "secret",
		Explicit:  "abcdef",
		Nested:    nested{Token: "nested"},
		Items:     []nested{{Token: "item"}},
		Headers:   value.Headers,
		Auto:      value.Auto,
		Integer:   -1,
		Unsigned:  2,
		Float:     3.5,
		Enabled:   true,
		ByteValue: []byte{1, 2},
	}, mode: maskAuto}).MarshalLogObject(enc); err != nil {
		t.Fatalf("auto struct encoding failed: %v", err)
	}
	if _, exists := enc.Fields["Skip"]; exists {
		t.Fatal("json:- field should be omitted in auto mode")
	}
	if _, exists := enc.Fields["empty"]; exists {
		t.Fatal("omitempty field should be omitted in auto mode")
	}
}

func TestAutoPayloadBranches(t *testing.T) {
	enc := zapcore.NewMapObjectEncoder()
	var nilInterface any
	var nilPointer *int
	var nilBytes []byte

	tests := []reflect.Value{
		{},
		reflect.ValueOf(&nilInterface).Elem(),
		reflect.ValueOf(nilPointer),
		reflect.ValueOf("plain"),
		reflect.ValueOf([]byte(`{"password":"secret"}`)),
		reflect.ValueOf(nilBytes),
		reflect.ValueOf(json.RawMessage(`{"token":"secret"}`)),
		reflect.ValueOf(&coverageJSONPointer{Email: "pointer@example.com"}),
	}
	for i, value := range tests {
		if err := addAutoPayload(enc, "value", value); err != nil {
			t.Fatalf("case %d failed: %v", i, err)
		}
	}

	if err := addAutoPayload(enc, "bad_raw", reflect.ValueOf(json.RawMessage(`invalid`))); err == nil {
		t.Fatal("expected invalid RawMessage error")
	}
	if got := maskJSONStringReflected(`invalid`); got != `invalid` {
		t.Fatalf("invalid reflected JSON changed to %q", got)
	}
}

func TestMaskedObjectDTOBranches(t *testing.T) {
	enc := zapcore.NewMapObjectEncoder()
	values := []any{
		HTTPData{}, (*HTTPData)(nil), &HTTPData{},
		DBData{}, (*DBData)(nil), &DBData{},
		MQData{}, (*MQData)(nil), &MQData{},
		GenericData{}, (*GenericData)(nil), &GenericData{},
		map[string][]string{"Cookie": {"secret"}},
		map[string]string{"password": "secret"},
		nilInterfaceValue(),
	}
	for i, value := range values {
		if err := (maskedObject{v: value, mode: maskHeaders}).MarshalLogObject(enc); err != nil {
			t.Fatalf("case %d failed: %v", i, err)
		}
	}
}

func nilInterfaceValue() any {
	var value any
	return value
}

func TestLargeMaskBufferAndPayloadErrors(t *testing.T) {
	input := `{"password":"` + strings.Repeat("x", maxPooledMaskJSONBuffer+1) + `"}`
	if got := maskJSONString(input); got != `{"password":"****"}` {
		t.Fatalf("large payload masking failed: %q", got)
	}
	if _, err := maskPayload(make(chan int)); err == nil {
		t.Fatal("expected channel marshal error")
	}
}
