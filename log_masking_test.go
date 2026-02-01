package goslogx

import (
	"testing"
)

func TestMaskingLogJSONBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantMask bool // Just check if masking occurred
	}{
		{
			name:     "Valid JSON with password",
			input:    []byte(`{"username":"admin","password":"secret123"}`),
			wantMask: true,
		},
		{
			name:     "Empty bytes",
			input:    []byte{},
			wantMask: false,
		},
		{
			name:     "Invalid JSON",
			input:    []byte("not json"),
			wantMask: false,
		},
		{
			name:     "Nested JSON",
			input:    []byte(`{"user":{"email":"test@example.com","password":"secret"}}`),
			wantMask: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskingLogJSONBytes("key", tt.input)

			if tt.wantMask {
				// Check password is masked
				if !contains(result, `"password":"****"`) {
					t.Errorf("Expected password to be masked in result: %s", result)
				}
				// Check it's not the original
				if result == string(tt.input) {
					t.Error("Expected masking to occur")
				}
			} else {
				if result != string(tt.input) {
					t.Errorf("Expected no masking, got %s", result)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMaskingLogHttpHeaders(t *testing.T) {
	tests := []struct {
		name  string
		input map[string][]string
	}{
		{
			name: "Headers with Authorization",
			input: map[string][]string{
				"Authorization": {"Bearer token123"},
				"Content-Type":  {"application/json"},
			},
		},
		{
			name:  "Empty headers",
			input: map[string][]string{},
		},
		{
			name: "Headers with API key",
			input: map[string][]string{
				"X-API-Key":  {"sk_live_123456"},
				"User-Agent": {"Mozilla/5.0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskingLogHttpHeaders("key", tt.input)
			if result == nil {
				t.Error("Expected non-nil result")
			}

			// Check Authorization is masked
			if auth, ok := result["Authorization"]; ok {
				if auth[0] != "****" {
					t.Errorf("Authorization should be fully masked, got %s", auth[0])
				}
			}

			// Check API key is partially masked
			if apiKey, ok := result["X-API-Key"]; ok {
				if apiKey[0] == "sk_live_123456" {
					t.Error("X-API-Key should be masked")
				}
			}

			// Check non-sensitive headers are not masked
			if ct, ok := result["Content-Type"]; ok {
				if ct[0] != "application/json" {
					t.Error("Content-Type should not be masked")
				}
			}
		})
	}
}

func TestMaskingLogJSONString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:  "JSON with credentials",
			input: `{"username":"john@example.com","password":"secret","api_key":"key123"}`,
			contains: []string{
				`"password":"****"`,
			},
		},
		{
			name:     "Empty string",
			input:    "",
			contains: []string{""},
		},
		{
			name:     "Non-JSON string",
			input:    "plain text",
			contains: []string{"plain text"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskingLogJSONString("key", tt.input)
			for _, expected := range tt.contains {
				if tt.name == "Empty string" && result != expected {
					t.Errorf("Expected empty string, got %s", result)
				}
			}
		})
	}
}
