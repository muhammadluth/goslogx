package goslogx

// MaskingLogJSONBytes parses a JSON byte slice and masks sensitive fields based on field names.
// It automatically detects and masks fields containing credentials, tokens, and personal information.
//
// Masking behavior:
//   - Full masking (****): password, secret, token, authorization, bearer
//   - Partial masking (first/last 2 chars): username, email, phone, api_key
//
// Returns the original string if the input is not valid JSON.
//
// Example:
//
//	body := []byte(`{"username":"admin@example.com","password":"secret123"}`)
//	masked := goslogx.MaskingLogJSONBytes("body", body)
//	// Result: {"username":"ad****om","password":"****"}
func MaskingLogJSONBytes(key string, data []byte) string {
	return maskJSONString(string(data))
}

// MaskingLogHttpHeaders masks sensitive values in HTTP headers.
// It detects sensitive headers like Authorization, Cookie, API keys and masks their values.
//
// Masking behavior:
//   - Full masking (****): Authorization, Bearer, Cookie
//   - Partial masking (first/last 2 chars): X-API-Key, API-Key
//
// Returns a new map with masked values. Non-sensitive headers are returned unchanged.
//
// Example:
//
//	headers := map[string][]string{
//	    "Authorization": {"Bearer token123"},
//	    "Content-Type":  {"application/json"},
//	}
//	masked := goslogx.MaskingLogHttpHeaders("headers", headers)
//	// Result: {"Authorization": ["****"], "Content-Type": ["application/json"]}
func MaskingLogHttpHeaders(key string, data map[string][]string) map[string][]string {
	return maskHttpHeaders(data)
}

// MaskingLogJSONString parses a JSON string and masks sensitive fields based on field names.
// This is a convenience wrapper around MaskingLogJSONBytes for string inputs.
//
// See MaskingLogJSONBytes for detailed masking behavior.
//
// Example:
//
//	json := `{"email":"user@example.com","secret_key":"sk_live_123"}`
//	masked := goslogx.MaskingLogJSONString("data", json)
//	// Result: {"email":"us****om","secret_key":"****"}
func MaskingLogJSONString(key string, data string) string {
	return maskJSONString(data)
}
