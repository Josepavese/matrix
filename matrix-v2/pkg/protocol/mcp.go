package protocol

// MCPRequest represents a Model Context Protocol interaction
type MCPRequest struct {
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}
