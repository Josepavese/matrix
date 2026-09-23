package zedacp

import (
	"errors"
	"strings"
)

// Identifier returns an authentication method's identifier whichever protocol
// generation supplied it, preferring methodId when both are present.
func (m AuthMethod) Identifier() string {
	if strings.TrimSpace(m.MethodID) != "" {
		return m.MethodID
	}
	return m.ID
}

// IsAuthenticationRequired reports whether a failure is the protocol's structured
// "authentication required" signal, introduced in ACP version 2 for requests gated
// behind a login.
//
// It exists so callers stop guessing from the text of an error message: before it,
// an authentication failure was recognised by looking for the substring "auth" in
// the message, which both missed structured signals and matched unrelated text. The
// string heuristic remains as a fallback for agents that only speak version 1 and
// never send the marker.
func IsAuthenticationRequired(err error) bool {
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || rpcErr == nil {
		return false
	}
	return rpcErr.Code == ErrCodeAuthenticationRequired || marksAuthenticationRequired(rpcErr.Data, 0)
}

const (
	// authRequiredSignal is the marker ACP v2 uses in the error data.
	authRequiredSignal = "auth_required"
	// maxAuthSignalDepth bounds the walk: Data comes from the agent process and is
	// therefore untrusted, so a deeply nested document must not be able to exhaust
	// the stack.
	maxAuthSignalDepth = 8
)

// marksAuthenticationRequired walks the error data looking for the marker. Any
// string anywhere in the payload counts, so the walk does not need to know which
// key an agent chose to put it under.
func marksAuthenticationRequired(data any, depth int) bool {
	if depth > maxAuthSignalDepth {
		return false
	}
	switch value := data.(type) {
	case string:
		return strings.Contains(strings.ToLower(value), authRequiredSignal)
	case map[string]any:
		for _, nested := range value {
			if marksAuthenticationRequired(nested, depth+1) {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if marksAuthenticationRequired(item, depth+1) {
				return true
			}
		}
	}
	return false
}
