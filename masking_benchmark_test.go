package goslogx_test

import (
	"testing"

	"github.com/muhammadluth/goslogx"
)

// Benchmark JSON masking functions
func BenchmarkMaskJSONString_Small(b *testing.B) {
	json := `{"username":"admin@example.com","password":"secret123"}`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		goslogx.MaskingLogJSONString("data", json)
	}
}

func BenchmarkMaskJSONString_Medium(b *testing.B) {
	json := `{
		"user": {
			"id": "user123",
			"username": "john.doe@example.com",
			"password": "supersecret",
			"profile": {
				"email": "john@company.com",
				"phone": "+1234567890",
				"api_key": "sk_live_1234567890abcdef"
			}
		},
		"session": {
			"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			"expires": "2024-12-31T23:59:59Z"
		}
	}`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		goslogx.MaskingLogJSONString("data", json)
	}
}

func BenchmarkMaskJSONString_Large(b *testing.B) {
	// Simulate large JSON with multiple users
	json := `{
		"users": [
			{"id":"1","username":"user1@example.com","password":"pass1","email":"user1@test.com"},
			{"id":"2","username":"user2@example.com","password":"pass2","email":"user2@test.com"},
			{"id":"3","username":"user3@example.com","password":"pass3","email":"user3@test.com"},
			{"id":"4","username":"user4@example.com","password":"pass4","email":"user4@test.com"},
			{"id":"5","username":"user5@example.com","password":"pass5","email":"user5@test.com"}
		],
		"metadata": {
			"total": 5,
			"api_key": "sk_live_abcdefghijklmnop",
			"secret_key": "secret_xyz123"
		}
	}`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		goslogx.MaskingLogJSONString("data", json)
	}
}

func BenchmarkMaskJSONBytes(b *testing.B) {
	data := []byte(`{"username":"admin","password":"secret","api_key":"key123"}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		goslogx.MaskingLogJSONBytes("data", data)
	}
}

func BenchmarkMaskHttpHeaders(b *testing.B) {
	headers := map[string][]string{
		"Authorization": {"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
		"Content-Type":  {"application/json"},
		"X-API-Key":     {"sk_live_1234567890abcdef"},
		"User-Agent":    {"Mozilla/5.0"},
		"Accept":        {"application/json"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		goslogx.MaskingLogHttpHeaders("headers", headers)
	}
}

// Benchmark struct masking
func BenchmarkStructMasking_Flat(b *testing.B) {
	type User struct {
		ID       string `json:"id"`
		Username string `json:"username" log:"masked:partial"`
		Password string `json:"password" log:"masked:full"`
		Email    string `json:"email" log:"masked:partial"`
	}

	user := User{
		ID:       "user123",
		Username: "johndoe",
		Password: "supersecret",
		Email:    "john@example.com",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		goslogx.Info("trace-001", "test", goslogx.MESSSAGE_TYPE_EVENT, "user data", user)
	}
}

func BenchmarkStructMasking_Nested(b *testing.B) {
	type Credentials struct {
		Username string `json:"username" log:"masked:partial"`
		Password string `json:"password" log:"masked:full"`
	}

	type User struct {
		ID          string      `json:"id"`
		Credentials Credentials `json:"credentials"`
		Email       string      `json:"email" log:"masked:partial"`
	}

	user := User{
		ID: "user123",
		Credentials: Credentials{
			Username: "johndoe",
			Password: "supersecret",
		},
		Email: "john@example.com",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		goslogx.Info("trace-001", "test", goslogx.MESSSAGE_TYPE_EVENT, "user data", user)
	}
}

func BenchmarkStructMasking_Slice(b *testing.B) {
	type User struct {
		ID       string `json:"id"`
		Username string `json:"username" log:"masked:partial"`
	}

	users := []User{
		{ID: "1", Username: "user1"},
		{ID: "2", Username: "user2"},
		{ID: "3", Username: "user3"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		goslogx.Info("trace-001", "test", goslogx.MESSSAGE_TYPE_EVENT, "users", users)
	}
}

// Benchmark HTTPData with masking
func BenchmarkHTTPData_WithSensitiveData(b *testing.B) {
	data := goslogx.HTTPData{
		Method:     "POST",
		URL:        "/api/v1/auth/login",
		StatusCode: 200,
		Headers: goslogx.MaskingLogHttpHeaders("headers", map[string][]string{
			"Authorization": {"Bearer token123"},
			"Content-Type":  {"application/json"},
		}),
		Body:     goslogx.MaskingLogJSONBytes("body", []byte(`{"username":"admin","password":"secret"}`)),
		Duration: "125ms",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		goslogx.Info("trace-001", "api", goslogx.MESSSAGE_TYPE_REQUEST, "request", data)
	}
}

func BenchmarkHTTPData_WithoutSensitiveData(b *testing.B) {
	data := goslogx.HTTPData{
		Method:     "GET",
		URL:        "/api/v1/users",
		StatusCode: 200,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
		Body:     "success",
		Duration: "45ms",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		goslogx.Info("trace-001", "api", goslogx.MESSSAGE_TYPE_REQUEST, "request", data)
	}
}
