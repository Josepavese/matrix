package protocol

// ACPMessage represents a standard Agent Communication Protocol request/response
type ACPMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}
