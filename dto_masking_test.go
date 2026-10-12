package goslogx

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDTOPayloadMasking(t *testing.T) {
	type credentials struct {
		Password string `json:"password"`
		Email    string `json:"email"`
		Count    int64  `json:"count"`
	}
	const input = `{"password":"secret123","email":"user@example.com","count":9007199254740993}`
	payloads := map[string]any{
		"string":  input,
		"bytes":   []byte(input),
		"raw":     json.RawMessage(input),
		"map":     map[string]any{"password": "secret123", "email": "user@example.com", "count": int64(9007199254740993)},
		"struct":  credentials{"secret123", "user@example.com", 9007199254740993},
		"pointer": &credentials{"secret123", "user@example.com", 9007199254740993},
		"nested":  map[string]any{"data": []credentials{{"secret123", "user@example.com", 9007199254740993}}},
	}
	for name, payload := range payloads {
		t.Run(name, func(t *testing.T) {
			before, _ := json.Marshal(payload)
			for _, dto := range []any{
				HTTPData{Body: payload, Headers: map[string][]string{"Authorization": {"Bearer secret123"}, "Cookie": {"secret123"}, "X-API-Key": {"abcdef"}}},
				DBData{Payload: payload}, MQData{Payload: payload}, GenericData{Payload: payload},
			} {
				ptr := reflect.New(reflect.TypeOf(dto))
				ptr.Elem().Set(reflect.ValueOf(dto))
				for _, value := range []any{dto, ptr.Interface()} {
					for _, level := range []string{"info", "debug", "warning"} {
						var out bytes.Buffer
						logger := setupLog(WithOutput(&out), WithDebug(true))
						switch level {
						case "info":
							logger.Info("trace", "test", MESSSAGE_TYPE_EVENT, "test", value)
						case "debug":
							logger.Debug("trace", "test", MESSSAGE_TYPE_EVENT, "test", value)
						case "warning":
							logger.Warning("trace", "test", "test", value)
						}
						result := out.String()
						if !json.Valid(bytes.TrimSpace(out.Bytes())) || strings.Contains(result, "secret123") || strings.Contains(result, "user@example.com") || !strings.Contains(result, "us****om") || !strings.Contains(result, "9007199254740993") {
							t.Fatalf("%T %s: unexpected log %s", value, level, result)
						}
					}
				}
			}
			after, _ := json.Marshal(payload)
			if !bytes.Equal(before, after) {
				t.Fatal("payload mutated")
			}
		})
	}
}

func TestMaskPayloadEdgeCases(t *testing.T) {
	for _, value := range []any{nil, (*int)(nil), []byte(nil), "not json", 42, true} {
		if _, err := maskPayload(value); err != nil {
			t.Fatalf("%T: %v", value, err)
		}
	}
	for _, value := range []any{make(chan int), json.RawMessage("invalid")} {
		if _, err := maskPayload(value); err == nil {
			t.Fatalf("%T: expected encoding error", value)
		}
	}
}

func TestMaskJSONStringFastPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{` { "password" : "secret", "profile" : { "email" : "user@example.com" }, "items" : [{"token":"abc"}, true, null, 123] } `, `{"password":"****","profile":{"email":"us****om"},"items":[{"token":"****"},true,null,123]}`},
		{`[]`, `[]`},
		{`{}`, `{}`},
		{`"plain"`, `"plain"`},
		{`123`, `123`},
		{`true`, `true`},
		{`null`, `null`},
		{`["abc", {"username":"abcd"}]`, `["abc",{"username":"****"}]`},
	}
	for _, tt := range tests {
		if got := maskJSONString(tt.input); got != tt.want {
			t.Errorf("maskJSONString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}

	for _, input := range []string{
		`{"password":"sec\\ret","email":"user@example.com"}`,
		`{"email":"pengguna@example.com","name":"José"}`,
		`{"value":"<html>"}`,
	} {
		if got, want := maskJSONString(input), maskJSONStringReflected(input); got != want {
			t.Errorf("fallback mismatch for %q: got %q, want %q", input, got, want)
		}
	}
}
