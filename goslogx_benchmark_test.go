package goslogx_test

import (
	"os"
	"testing"

	"github.com/muhammadluth/goslogx"
)

func init() {
	// Open /dev/null for the logger
	f, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0666)

	// Save original stdout
	oldStdout := os.Stdout

	// Swap stdout so New picks up the null writer
	os.Stdout = f

	// Initialize global logger
	goslogx.New(
		goslogx.WithServiceName("bench-service"),
		goslogx.WithOutput(f),
	)

	// Restore stdout for test runner
	os.Stdout = oldStdout
}

func BenchmarkInfoNoData(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goslogx.Info("trace-001", "api", goslogx.MESSSAGE_TYPE_EVENT, "request received", nil)
	}
}

func BenchmarkInfoWithDTO(b *testing.B) {
	data := &goslogx.HTTPData{
		Method:     "GET",
		URL:        "/api/v1/users",
		StatusCode: 200,
		Headers: map[string][]string{
			"Authorization": {"Bearer xyz"},
			"Content-Type":  {"application/json"},
		},
		Body: "test",
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goslogx.Info("trace-001", "api", goslogx.MESSSAGE_TYPE_EVENT, "request received", data)
	}
}

func BenchmarkError(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goslogx.Error("trace-001", "api", &CustomError{Msg: "internal server error"})
	}
}

func BenchmarkDebugNoData(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goslogx.Debug("trace-001", "api", goslogx.MESSSAGE_TYPE_EVENT, "debug message", nil)
	}
}

func BenchmarkDebugWithDTO(b *testing.B) {
	data := &goslogx.HTTPData{
		Method:     "GET",
		URL:        "/api/v1/users",
		StatusCode: 200,
		Headers: map[string][]string{
			"Authorization": {"Bearer xyz"},
			"Content-Type":  {"application/json"},
		},
		Body: "test",
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goslogx.Debug("trace-001", "api", goslogx.MESSSAGE_TYPE_EVENT, "debug message", data)
	}
}

func BenchmarkDebugWithNestedMasking(b *testing.B) {
	type UserAuth struct {
		Email    string `json:"email" log:"masked:partial"`
		Password string `json:"password" log:"masked:full"`
		Name     string `json:"name"`
	}

	type User struct {
		ID       string   `json:"id"`
		UserAuth UserAuth `json:"user_auth"`
	}

	user := User{
		ID: "user-123",
		UserAuth: UserAuth{
			Email:    "john.doe@example.com",
			Password: "supersecret",
			Name:     "John Doe",
		},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goslogx.Debug("trace-001", "auth", goslogx.MESSSAGE_TYPE_EVENT, "user debug", user)
	}
}

func BenchmarkDebugWithSliceMasking(b *testing.B) {
	type Admin struct {
		ID       string `json:"id"`
		Username string `json:"username" log:"masked:partial"`
		Name     string `json:"name"`
	}

	admins := []Admin{
		{ID: "admin-001", Username: "johndoe123", Name: "John Doe"},
		{ID: "admin-002", Username: "janesmith456", Name: "Jane Smith"},
		{ID: "admin-003", Username: "bobwilson789", Name: "Bob Wilson"},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goslogx.Debug("trace-001", "auth", goslogx.MESSSAGE_TYPE_EVENT, "admin debug", admins)
	}
}

func BenchmarkWarningNoData(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goslogx.Warning("trace-001", "cache", "high latency detected", nil)
	}
}

func BenchmarkWarningWithDTO(b *testing.B) {
	data := map[string]interface{}{
		"latency_ms": 450.5,
		"threshold":  300,
		"service":    "cache-redis",
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goslogx.Warning("trace-001", "cache", "high latency detected", data)
	}
}

// Benchmark nested struct with masking
func BenchmarkInfoWithNestedMasking(b *testing.B) {
	type UserAuth struct {
		Email    string `json:"email" log:"masked:partial"`
		Password string `json:"password" log:"masked:full"`
		Name     string `json:"name"`
	}

	type User struct {
		ID       string   `json:"id"`
		UserAuth UserAuth `json:"user_auth"`
	}

	user := User{
		ID: "user-123",
		UserAuth: UserAuth{
			Email:    "john.doe@example.com",
			Password: "supersecret",
			Name:     "John Doe",
		},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goslogx.Info("trace-001", "auth", goslogx.MESSSAGE_TYPE_EVENT, "user login", user)
	}
}

// Benchmark slice of structs with masking
func BenchmarkInfoWithSliceMasking(b *testing.B) {
	type Admin struct {
		ID       string `json:"id"`
		Username string `json:"username" log:"masked:partial"`
		Name     string `json:"name"`
	}

	admins := []Admin{
		{ID: "admin-001", Username: "johndoe123", Name: "John Doe"},
		{ID: "admin-002", Username: "janesmith456", Name: "Jane Smith"},
		{ID: "admin-003", Username: "bobwilson789", Name: "Bob Wilson"},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goslogx.Info("trace-001", "auth", goslogx.MESSSAGE_TYPE_EVENT, "admin list", admins)
	}
}

// Custom error untuk benchmark
type CustomError struct {
	Msg string
}

func (e *CustomError) Error() string { return e.Msg }
