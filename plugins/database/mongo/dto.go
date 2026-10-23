package mongo

type ConnectionKey struct {
	Address string `json:"address"`
	ID      int64  `json:"id"`
}
