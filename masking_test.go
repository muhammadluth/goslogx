package goslogx_test

import (
	"testing"
	"time"

	"github.com/muhammadluth/goslogx"
)

// Test nested struct masking
func TestNestedStructMasking(t *testing.T) {
	goslogx.New(
		goslogx.WithServiceName("test-service"),
		goslogx.WithMasking(true),
	)

	type UserAuth struct {
		Username string `json:"username" log:"masked:partial"`
		Email    string `json:"email" log:"masked:partial"`
		Password string `json:"password" log:"masked:full"`
	}

	type User struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		UserAuth UserAuth `json:"user_auth"`
	}

	type TransactionData struct {
		ID              string    `json:"id"`
		TransactionDate time.Time `json:"transaction_date"`
		User            User      `json:"user"`
	}

	data := TransactionData{
		ID:              "TXN-001",
		TransactionDate: time.Now(),
		User: User{
			ID:   "USR-001",
			Name: "John Doe",
			UserAuth: UserAuth{
				Username: "johndoe123",
				Email:    "john.doe@example.com",
				Password: "supersecret",
			},
		},
	}

	// This should log with nested masking applied
	goslogx.Info("trace-001", "transaction", goslogx.MESSSAGE_TYPE_EVENT, "transaction created", data)

	t.Log("Nested struct masking test completed - check logs manually")
}

// Test pointer to nested struct
func TestPointerNestedStruct(t *testing.T) {
	goslogx.New(
		goslogx.WithServiceName("test-service"),
		goslogx.WithMasking(true),
	)

	type Credentials struct {
		APIKey string `json:"api_key" log:"masked:full"`
		Secret string `json:"secret" log:"masked:full"`
	}

	type Service struct {
		Name        string       `json:"name"`
		Credentials *Credentials `json:"credentials"`
	}

	data := Service{
		Name: "PaymentGateway",
		Credentials: &Credentials{
			APIKey: "sk_live_1234567890",
			Secret: "secret_key_abcdef",
		},
	}

	goslogx.Info("trace-002", "service", goslogx.MESSSAGE_TYPE_EVENT, "service initialized", data)

	t.Log("Pointer nested struct masking test completed")
}
