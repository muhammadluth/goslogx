package goslogx

type MsgType string

const (
	MESSSAGE_TYPE_IN       MsgType = "IN"
	MESSSAGE_TYPE_OUT      MsgType = "OUT"
	MESSSAGE_TYPE_REQUEST  MsgType = "REQUEST"
	MESSSAGE_TYPE_RESPONSE MsgType = "RESPONSE"
)
