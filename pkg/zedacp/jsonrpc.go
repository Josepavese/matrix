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
	// ErrCodeInvalidRequest is returned when an inbound frame cannot be decoded
	// into a request at all.
	ErrCodeInvalidRequest = -32600
	// ErrCodeAuthenticationRequired is ACP's "authentication is required before
	// this operation can be performed". Both generations define it, so a gated
	// request is recognised from the code the specification assigns it and not
	// only from the shape of the error data.
	ErrCodeAuthenticationRequired = -32000
	// ErrCodeResourceNotFound is ACP's "a given resource, such as a file, was
	// not found", which is how both generations report a session that is gone.
	ErrCodeResourceNotFound = -32002
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

// rpcErrorFromWire rebuilds the typed error an inbound JSON-RPC error response
// carries. It keeps the exact text an inbound error always produced, and what it
// adds is that the error stays typed: ACP version 2 puts structured signals in
// the data field — "auth_required" above all — and a caller cannot recognize
// what a flattened string no longer carries.
func rpcErrorFromWire(err *jsonRPCError) error {
	if err == nil {
		return nil
	}
	text := fmt.Sprintf("RPC error %d: %s", err.Code, err.Message)
	if err.Data != nil {
		text = fmt.Sprintf("%s (%v)", text, err.Data)
	}
	return &RPCError{Code: err.Code, Message: text, Data: err.Data}
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
