package goslogx

// MsgType represents the classification of a log message.
// It indicates whether the message represents incoming data, outgoing data,
// a request, a response, or an application event.
type MsgType string

const (
	// MESSSAGE_TYPE_IN indicates an incoming message or data
	MESSSAGE_TYPE_IN MsgType = "IN"
	// MESSSAGE_TYPE_OUT indicates an outgoing message or data
	MESSSAGE_TYPE_OUT MsgType = "OUT"
	// MESSSAGE_TYPE_REQUEST indicates an outgoing request to external service
	MESSSAGE_TYPE_REQUEST MsgType = "REQUEST"
	// MESSSAGE_TYPE_RESPONSE indicates an incoming response from external service
	MESSSAGE_TYPE_RESPONSE MsgType = "RESPONSE"
	// MESSSAGE_TYPE_EVENT indicates an application event
	MESSSAGE_TYPE_EVENT MsgType = "EVENT"
)
