package goslogx

// HTTPRequestData represents context for HTTP interactions
type HTTPRequestData struct {
	Method     string              `json:"method,omitempty"`
	URL        string              `json:"url,omitempty"`
	StatusCode int                 `json:"status_code,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       any                 `json:"body,omitempty"`
	ClientIP   string              `json:"client_ip,omitempty"`
}

// DBData represents context for Database/Cache interactions (Redis, SQL, etc.)
type DBData struct {
	Driver     string `json:"driver,omitempty"`
	Operation  string `json:"operation,omitempty"`
	Table      string `json:"table,omitempty"`
	Statement  string `json:"statement,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

// MQData represents context for Message Queue interactions (Nats, Kafka, etc.)
type MQData struct {
	Driver    string `json:"driver,omitempty"`
	Operation string `json:"operation,omitempty"`
	Topic     string `json:"topic,omitempty"`
	Group     string `json:"group,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	Payload   any    `json:"payload,omitempty"`
}

// GenericData for any other context
type GenericData struct {
	Service string `json:"service,omitempty"`
	Action  string `json:"action,omitempty"`
	Payload any    `json:"payload,omitempty"`
}
