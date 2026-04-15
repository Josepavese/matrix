package agents

import (
	"context"
	"encoding/json"

	"github.com/jose/matrix-v2/internal/middleware"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  *string         `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

type ACPClient = acpClient

func NewACPClient(ctx context.Context, transport middleware.AgentTransport) ACPClient {
	return defaultACPSDK.NewClient(ctx, transport)
}
