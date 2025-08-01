package goslogx

type HttpDynamicData struct {
	Host        string `json:"host,omitempty"`
	Method      string `json:"method,omitempty"`
	Path        string `json:"path,omitempty"`
	Type        string `json:"type,omitempty"`
	StatusCode  int    `json:"status_code,omitempty"`
	HttpMessage any    `json:"http_message,omitempty"`
}

type NatsDynamicData struct {
	Name        string              `json:"name,omitempty"`
	Type        string              `json:"type,omitempty"`
	Subject     string              `json:"subject,omitempty"`
	Stream      string              `json:"stream,omitempty"`
	Consumer    string              `json:"consumer,omitempty"`
	NatsHeader  map[string][]string `json:"nats_header,omitempty"`
	NatsMessage any                 `json:"nats_message,omitempty"`
}

type RedisDynamicData struct {
	Name         string   `json:"name,omitempty"`
	Type         string   `json:"type,omitempty"`
	Database     int      `json:"database"`
	Key          string   `json:"key,omitempty"`
	Fields       []string `json:"fields,omitempty"`
	RedisMessage any      `json:"redis_message,omitempty"`
}

type XenditDynamicData struct {
	MessageType string `json:"message_type,omitempty"`
	Message     any    `json:"message,omitempty"`
}
