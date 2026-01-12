package goslogx

// HTTPRequestData represents context information for HTTP interactions.
// It captures details about HTTP requests and responses including
// method, URL, status code, headers, body, and client information.
//
// Example:
//
//	data := goslogx.HTTPRequestData{
//		Method:     "POST",
//		URL:        "/api/v1/users",
//		StatusCode: 201,
//		ClientIP:   "192.168.1.1",
//	}
//	goslogx.Info(ctx, "trace-001", "http", goslogx.MESSSAGE_TYPE_REQUEST, "request completed", data)
type HTTPRequestData struct {
	Method     string              `json:"method,omitempty"`
	URL        string              `json:"url,omitempty"`
	StatusCode int                 `json:"status_code,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       any                 `json:"body,omitempty"`
	ClientIP   string              `json:"client_ip,omitempty"`
}

// DBData represents context information for Database/Cache interactions.
// It captures details about database operations including driver type,
// operation type, table/key information, SQL statements, and performance metrics.
// Supports SQL databases, NoSQL databases, and cache systems like Redis.
//
// Example:
//
//	data := goslogx.DBData{
//		Driver:     "postgres",
//		Operation:  "SELECT",
//		Table:      "users",
//		Statement:  "SELECT * FROM users WHERE id = $1",
//		DurationMs: 45,
//	}
//	goslogx.Info(ctx, "trace-001", "database", goslogx.MESSSAGE_TYPE_IN, "query executed", data)
type DBData struct {
	Driver     string `json:"driver,omitempty"`
	Operation  string `json:"operation,omitempty"`
	Table      string `json:"table,omitempty"`
	Statement  string `json:"statement,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

// MQData represents context information for Message Queue interactions.
// It captures details about message queue operations including
// broker type, operation, topic/queue, consumer group, and message information.
// Supports various message brokers like Kafka, RabbitMQ, NATS, etc.
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
//	goslogx.Info(ctx, "trace-001", "messaging", goslogx.MESSSAGE_TYPE_IN, "message received", data)
type MQData struct {
	Driver    string `json:"driver,omitempty"`
	Operation string `json:"operation,omitempty"`
	Topic     string `json:"topic,omitempty"`
	Group     string `json:"group,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	Payload   any    `json:"payload,omitempty"`
}

// GenericData is a flexible data structure for logging context of any external service interaction.
// Use this for third-party service calls that don't fit into HTTP, Database, or Message Queue categories.
// Useful for payments, SMS services, email, or any custom external APIs.
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
//	goslogx.Info(ctx, "trace-001", "payment", goslogx.MESSSAGE_TYPE_REQUEST, "charge initiated", data)
type GenericData struct {
	Service string `json:"service,omitempty"`
	Action  string `json:"action,omitempty"`
	Payload any    `json:"payload,omitempty"`
}
