package zedacp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  *string         `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	ErrCodeMethodNotFound = -32601
	ErrCodeInternal       = -32603
)

// RPCError lets request handlers return protocol-correct JSON-RPC error codes.
type RPCError struct {
	Code    int
	Message string
	Data    any
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("RPC error %d", e.Code)
}

func NewMethodNotFoundError(method string) error {
	method = strings.TrimSpace(method)
	if method == "" {
		method = "<unknown>"
	}
	return &RPCError{Code: ErrCodeMethodNotFound, Message: "method not found: " + method}
}

func rpcErrorFromError(err error) *jsonRPCError {
	if err == nil {
		return nil
	}
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) && rpcErr != nil {
		return &jsonRPCError{Code: rpcErr.Code, Message: rpcErr.Error(), Data: rpcErr.Data}
	}
	return &jsonRPCError{Code: ErrCodeInternal, Message: err.Error()}
}

func newJSONRPCID(id int64) json.RawMessage {
	return json.RawMessage(strconv.FormatInt(id, 10))
}

func jsonRPCIDInt64(id json.RawMessage) (int64, bool) {
	if len(id) == 0 {
		return 0, false
	}
	decoder := json.NewDecoder(bytes.NewReader(id))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return 0, false
	}
	switch typed := value.(type) {
	case json.Number:
		out, err := typed.Int64()
		return out, err == nil
	case string:
		out, err := strconv.ParseInt(typed, 10, 64)
		return out, err == nil
	default:
		return 0, false
	}
}

func jsonRPCIDLogValue(id json.RawMessage) interface{} {
	if len(id) == 0 {
		return nil
	}
	var value interface{}
	if err := json.Unmarshal(id, &value); err == nil {
		return value
	}
	return string(id)
}

func cloneRawMessage(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), data...)
}
