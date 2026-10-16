package main

import (
	"fmt"
	"time"

	log "github.com/muhammadluth/goslogx"
)

func main() {
	// Initialize logger
	log.New(
		log.WithServiceName("api-gateway-demo"),
		log.WithMasking(true),
	)

	// Simulate HTTP request with sensitive data
	requestBody := `{
		"username": "admin@company.com",
		"password": "SuperSecret123!",
		"email": "admin@company.com",
		"api_key": "sk_live_1234567890abcdef"
	}`

	log.Info(
		"trace-12345",
		"http-middleware",
		log.MESSSAGE_TYPE_REQUEST,
		"Incoming login request",
		log.HTTPData{
			Method:     "POST",
			URL:        "/api/v1/auth/login",
			StatusCode: 0, // Not yet processed
			Headers: log.MaskingLogHttpHeaders("headers", map[string][]string{
				"Content-Type":  {"application/json"},
				"Authorization": {"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0"},
				"X-API-Key":     {"sk_live_abcdefghijklmnop"},
				"User-Agent":    {"Mozilla/5.0"},
			}),
			Body:     log.MaskingLogJSONBytes("body", []byte(requestBody)),
			ClientIP: "203.0.113.42",
		},
	)

	// Simulate processing
	time.Sleep(125 * time.Millisecond)

	// Simulate response
	responseBody := `{
		"success": true,
		"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.newtoken",
		"user_id": "usr_123456",
		"email": "admin@company.com"
	}`

	log.Info(
		"trace-12345",
		"http-middleware",
		log.MESSSAGE_TYPE_RESPONSE,
		"Login successful",
		log.HTTPData{
			Method:     "POST",
			URL:        "/api/v1/auth/login",
			StatusCode: 200,
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
				"Set-Cookie":   {"session=abc123; HttpOnly; Secure"},
			},
			Body:     responseBody,
			Duration: "125ms",
			ClientIP: "203.0.113.42",
		},
	)

	fmt.Println("\n✅ Check the logs above:")
	fmt.Println("   - password field should be fully masked (****)")
	fmt.Println("   - username/email should be partially masked (ad****om)")
	fmt.Println("   - Authorization header should be fully masked")
	fmt.Println("   - X-API-Key should be partially masked")
	fmt.Println("   - token in response should be fully masked")
}
