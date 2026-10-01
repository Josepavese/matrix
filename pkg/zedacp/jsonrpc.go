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
//
// Message is the error's own text, verbatim: joining it to the code is what
// Error does, and a peer's message that a caller reads programmatically is not
// prefixed with a rendering of itself.
type RPCError struct {
	Code    int
	Message string
	Data    any
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return renderRPCError(e.Code, e.Message, e.Data)
}

// RPCErrorCode, RPCErrorMessage and RPCErrorData expose the three fields a
// consumer that only knows "there is a coded protocol error here" needs. They
// exist because the fields themselves carry no interface: a middleware layer
// that must not import a protocol SDK can still read a peer's code, message and
// payload without unwrapping a rendered string.
func (e *RPCError) RPCErrorCode() int {
	if e == nil {
		return 0
	}
	return e.Code
}

func (e *RPCError) RPCErrorMessage() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *RPCError) RPCErrorData() any {
	if e == nil {
		return nil
	}
	return e.Data
}

// ErrTextInternal is the protocol's own word for an error with no message of its
// own. It substitutes for an absent message, never for one that was sent.
const ErrTextInternal = "Internal error"

// renderRPCError is the one-line rendering of a JSON-RPC error: its code, its
// text, and its structured payload when there is one.
func renderRPCError(code int, message string, data any) string {
	text := strings.TrimSpace(message)
	if text == "" {
		text = ErrTextInternal
	}
	rendered := fmt.Sprintf("RPC error %d: %s", code, text)
	if payload := formatRPCErrorData(data); payload != "" {
		rendered = fmt.Sprintf("%s (%s)", rendered, payload)
	}
	return rendered
}

// formatRPCErrorData renders an error's structured payload for a human reading
// one line, and returns nothing when there is no payload to render.
//
// An empty object or empty list is not a payload: JSON-RPC makes "data" optional,
// and peers routinely send `"data": {}` to mean "no additional context". Printing
// it produced the suffix "(map[])", which told an operator nothing and hid the
// message it was appended to. The value itself is not lost by this: it stays on
// RPCError.Data for a consumer that reads it programmatically.
func formatRPCErrorData(data any) string {
	if emptyRPCErrorData(data) {
		return ""
	}
	return fmt.Sprintf("%v", data)
}

// emptyRPCErrorData reports whether a structured payload carries no content.
func emptyRPCErrorData(data any) bool {
	switch typed := data.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case map[string]any:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	default:
		return false
	}
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
		// The message travels as itself: the wire frame carries code and data as
		// their own fields, and a caller reading the text of the rebuilt error
		// gets them rendered once rather than twice.
		return &jsonRPCError{Code: rpcErr.Code, Message: rpcErr.Message, Data: rpcErr.Data}
	}
	return &jsonRPCError{Code: ErrCodeInternal, Message: err.Error()}
}

// rpcErrorFromWire rebuilds the typed error an inbound JSON-RPC error response
// carries. The code, the peer's message and its payload each stay their own
// field rather than only a pre-rendered string, so the diagnostic survives the
// boundary instead of being recoverable only by parsing prose back apart — and
// so reading the error renders it once, whichever path produced it.
//
// An empty payload changes nothing: JSON-RPC makes "data" optional, and a peer
// that sends `{}` means "no additional context". The `(map[])` an operator used
// to see was Matrix rendering that emptiness as if it were evidence.
func rpcErrorFromWire(err *jsonRPCError) error {
	if err == nil {
		return nil
	}
	return &RPCError{Code: err.Code, Message: err.Message, Data: err.Data}
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
