package goslogx

import (
	"reflect"
	"testing"
)

// Test maskJSONValue for all types
func TestMaskJSONValue(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{
			name:  "Map with sensitive fields",
			input: map[string]interface{}{"password": "secret", "username": "admin"},
		},
		{
			name:  "Array of maps",
			input: []interface{}{map[string]interface{}{"password": "secret"}},
		},
		{
			name:  "Array of strings",
			input: []interface{}{"value1", "value2"},
		},
		{
			name:  "String value",
			input: "plain string",
		},
		{
			name:  "Number value",
			input: 123.45,
		},
		{
			name:  "Boolean value",
			input: true,
		},
		{
			name:  "Nil value",
			input: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskJSONValue(tt.input)
			if result == nil && tt.input != nil {
				t.Error("Expected non-nil result")
			}
		})
	}
}

// Test dataField edge cases
func TestDataFieldEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value interface{}
	}{
		{
			name:  "Nil value",
			key:   "key",
			value: nil,
		},
		{
			name:  "String value",
			key:   "key",
			value: "test string",
		},
		{
			name:  "Int value",
			key:   "key",
			value: 123,
		},
		{
			name:  "Float value",
			key:   "key",
			value: 123.45,
		},
		{
			name:  "Bool value",
			key:   "key",
			value: true,
		},
		{
			name:  "Empty slice",
			key:   "key",
			value: []string{},
		},
		{
			name:  "Slice of primitives",
			key:   "key",
			value: []int{1, 2, 3},
		},
		{
			name:  "Map value",
			key:   "key",
			value: map[string]string{"key": "value"},
		},
		{
			name:  "Nil pointer",
			key:   "key",
			value: (*string)(nil),
		},
		{
			name: "Slice of structs",
			key:  "key",
			value: []struct {
				Name string
			}{{Name: "test"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := dataField(tt.key, tt.value)

			// For nil values, dataField returns zap.Skip() which has empty key
			if tt.value == nil || (reflect.ValueOf(tt.value).Kind() == reflect.Ptr && reflect.ValueOf(tt.value).IsNil()) {
				if field.Key != "" {
					t.Errorf("Expected empty key for nil value, got %s", field.Key)
				}
			} else {
				if field.Key != tt.key {
					t.Errorf("Expected key %s, got %s", tt.key, field.Key)
				}
			}
		})
	}
}

// Test getStructMeta with various struct types
func TestGetStructMetaEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		value  interface{}
		fields int
	}{
		{
			name: "Struct with unexported fields",
			value: struct {
				Exported   string
				unexported string
			}{},
			fields: 1, // Only exported field
		},
		{
			name: "Struct with json tags",
			value: struct {
				Field1 string `json:"field_1"`
				Field2 string `json:"field_2,omitempty"`
				Field3 string `json:"-"`
			}{},
			fields: 3, // All exported fields are included, json:"-" just changes the name
		},
		{
			name: "Struct with masking tags",
			value: struct {
				Field1 string `log:"masked:full"`
				Field2 string `log:"masked:partial"`
				Field3 string
			}{},
			fields: 3,
		},
		{
			name:   "Empty struct",
			value:  struct{}{},
			fields: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := reflect.TypeOf(tt.value)
			meta := getStructMeta(typ)
			if len(meta.fields) != tt.fields {
				t.Errorf("Expected %d fields, got %d", tt.fields, len(meta.fields))
			}

			// Test cache hit
			meta2 := getStructMeta(typ)
			if meta != meta2 {
				t.Error("Expected cached metadata")
			}
		})
	}
}

// Test maskJSONString edge cases
func TestMaskJSONStringEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Empty string",
			input: "",
		},
		{
			name:  "Invalid JSON",
			input: "{invalid json}",
		},
		{
			name:  "JSON array",
			input: `[{"password":"secret"}]`,
		},
		{
			name:  "Nested JSON",
			input: `{"user":{"credentials":{"password":"secret"}}}`,
		},
		{
			name:  "JSON with null values",
			input: `{"password":null,"username":"admin"}`,
		},
		{
			name:  "JSON with numbers",
			input: `{"password":"secret","age":25}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskJSONString(tt.input)
			if tt.input == "" && result != "" {
				t.Error("Expected empty result for empty input")
			}
			if tt.input == "{invalid json}" && result != tt.input {
				t.Error("Expected original string for invalid JSON")
			}
		})
	}
}

// Test maskJSONMap edge cases
func TestMaskJSONMapEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]interface{}
	}{
		{
			name:  "Empty map",
			input: map[string]interface{}{},
		},
		{
			name: "Map with nested maps",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"password": "secret",
				},
			},
		},
		{
			name: "Map with arrays",
			input: map[string]interface{}{
				"users": []interface{}{
					map[string]interface{}{"password": "secret1"},
					map[string]interface{}{"password": "secret2"},
				},
			},
		},
		{
			name: "Map with mixed types",
			input: map[string]interface{}{
				"password": "secret",
				"age":      25,
				"active":   true,
				"data":     nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskJSONMap(tt.input)
			if result == nil {
				t.Error("Expected non-nil result")
			}
		})
	}
}

// Test shouldMaskField with various patterns
func TestShouldMaskFieldPatterns(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected maskType
	}{
		// Full masking
		{"password field", "password", maskFull},
		{"passwd field", "passwd", maskFull},
		{"secret field", "secret_key", maskFull},
		{"token field", "access_token", maskFull},
		{"authorization field", "authorization", maskFull},
		{"bearer field", "bearer", maskFull},
		{"credential field", "user_credential", maskFull},

		// Partial masking
		{"username field", "username", maskPartial},
		{"email field", "user_email", maskPartial},
		{"phone field", "phone_number", maskPartial},
		{"api_key field", "api_key", maskPartial},
		{"access_key field", "access_key", maskPartial},

		// No masking
		{"normal field", "normal_field", maskNone},
		{"id field", "user_id", maskPartial}, // user_id contains "user" pattern
		{"name field", "full_name", maskNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldMaskField(tt.field)
			if result != tt.expected {
				t.Errorf("shouldMaskField(%s) = %v, want %v", tt.field, result, tt.expected)
			}
		})
	}
}

// Test maskMiddle edge cases
func TestMaskMiddleEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Short string", "abc", "****"},
		{"4 char string", "abcd", "****"},
		{"5 char string", "abcde", "ab****de"},
		{"Long string", "verylongstring", "ve****ng"},
		{"Empty string", "", "****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskMiddle(tt.input)
			if result != tt.expected {
				t.Errorf("maskMiddle(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}
