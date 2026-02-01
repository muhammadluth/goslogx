package goslogx

// HTTPData captures context for HTTP interactions.
// It provides a structured schema for logging request/response and client metadata.
// Sensitive fields in Body (JSON) and Headers are automatically masked.
//
// Example:
//
//	data := goslogx.HTTPData{
//		Method:     "POST",
//		URL:        "https://example.com/api/v1/users",
//		StatusCode: 201,
//		ClientIP:   "192.168.1.1",
//	}
//	goslogx.Info("trace-001", "http", goslogx.MESSSAGE_TYPE_REQUEST, "request completed", data)
type HTTPData struct {
	Method     string              `json:"method,omitempty"`
	URL        string              `json:"url,omitempty"`
	StatusCode int                 `json:"status_code,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       any                 `json:"body,omitempty"`
	Duration   string              `json:"duration,omitempty"`
	ClientIP   string              `json:"client_ip,omitempty"`
}

// DBData captures context for database or cache operations.
// It tracks the driver, operation, and execution duration.
//
// Example:
//
//	data := goslogx.DBData{
//		Driver:     "postgres",
//		Operation:  "SELECT",
//		Database:   "postgres",
//		Table:      "users",
//		Statement:  "SELECT * FROM users WHERE id = $1",
//		Duration:   "45ms",
//	}
//	goslogx.Info("trace-001", "database", goslogx.MESSSAGE_TYPE_IN, "query executed", data)
type DBData struct {
	Driver    string `json:"driver,omitempty"`
	Operation string `json:"operation,omitempty"`
	Database  string `json:"database,omitempty"`
	Table     string `json:"table,omitempty"`
	Statement string `json:"statement,omitempty"`
	Duration  string `json:"duration,omitempty"`
	Payload   any    `json:"payload,omitempty"`
}

// MQData captures context for Message Queue interactions.
// It is compatible with Kafka, RabbitMQ, NATS, etc.
//
// Example:
//
//	data := goslogx.MQData{
//		Driver:    "kafka",
//		Operation: "consume",
//		Topic:     "user-events",
//		Group:     "notification-service",
//		MessageID: "msg-123",
//	}
//	goslogx.Info("trace-001", "messaging", goslogx.MESSSAGE_TYPE_IN, "message received", data)
type MQData struct {
	Driver    string `json:"driver,omitempty"`
	Operation string `json:"operation,omitempty"`
	Topic     string `json:"topic,omitempty"`
	Group     string `json:"group,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	Payload   any    `json:"payload,omitempty"`
}

// GenericData provides a flexible schema for logging interactions with external
// services that do not fit into standard HTTP, DB, or MQ categories.
//
// Example:
//
//	data := goslogx.GenericData{
//		Service: "Stripe",
//		Action:  "Charge",
//		Payload: map[string]interface{}{
//			"amount":   100,
//			"currency": "USD",
//		},
//	}
//	goslogx.Info("trace-001", "payment", goslogx.MESSSAGE_TYPE_REQUEST, "charge initiated", data)
type GenericData struct {
	Service string `json:"service,omitempty"`
	Action  string `json:"action,omitempty"`
	Payload any    `json:"payload,omitempty"`
}
