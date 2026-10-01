package zedacp

import (
	"errors"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------------
// JSON-RPC error diagnostics across the wire boundary
// ----------------------------------------------------------------------------

// TestEmptyErrorDataDoesNotBecomeMapText is the regression guard for the
// "-32603 Internal error (map[])" an operator saw. The peer had sent
// `"data": {}` with the message "Internal error"; Matrix appended the rendering
// of an empty map to it. An absent payload must add nothing, and the diagnostic
// the peer did send — its code and its message — must survive intact.
func TestEmptyErrorDataDoesNotBecomeMapText(t *testing.T) {
	rebuilt := rpcErrorFromWire(&jsonRPCError{
		Code:    ErrCodeInternal,
		Message: "Internal error",
		Data:    map[string]any{},
	})
	if rebuilt == nil {
		t.Fatal("expected a typed error")
	}
	text := rebuilt.Error()
	if strings.Contains(text, "map[]") {
		t.Fatalf("an empty data payload was rendered as text: %q", text)
	}
	if !strings.Contains(text, "Internal error") {
		t.Fatalf("the peer's message was lost: %q", text)
	}
	if !strings.Contains(text, "-32603") {
		t.Fatalf("the peer's code was lost: %q", text)
	}

	var typed *RPCError
	if !errors.As(rebuilt, &typed) {
		t.Fatalf("expected the error to stay typed, got %T", rebuilt)
	}
	if typed.Code != ErrCodeInternal {
		t.Fatalf("structured code = %d, want %d", typed.Code, ErrCodeInternal)
	}
	if typed.Message != "Internal error" {
		t.Fatalf("structured message = %q, want %q", typed.Message, "Internal error")
	}
}

// TestErrorDataThatCarriesContentIsStillReported is the counterweight: the rule
// that drops an empty payload must not drop a payload that says something. ACP
// version 2 puts structured signals in that field, so a peer that sent one still
// has it in the text a caller reads.
func TestErrorDataThatCarriesContentIsStillReported(t *testing.T) {
	rebuilt := rpcErrorFromWire(&jsonRPCError{
		Code:    ErrCodeAuthenticationRequired,
		Message: "authentication required",
		Data:    map[string]any{"kind": "auth_required"},
	})
	if rebuilt == nil {
		t.Fatal("expected a typed error")
	}
	text := rebuilt.Error()
	if !strings.Contains(text, "auth_required") {
		t.Fatalf("a structured payload was dropped from the text: %q", text)
	}
	if !strings.Contains(text, "authentication required") {
		t.Fatalf("the peer's message was lost: %q", text)
	}
	if !IsAuthenticationRequired(rebuilt) {
		t.Fatalf("the typed signal was lost: %q", text)
	}
}

// TestLocallyBuiltErrorKeepsCodeAndMessage closes the second path to the same
// bug: an error Matrix builds itself rendered an empty payload the same way, and
// dropped its own code and message when it carried none. It is hygiene for a
// path no reproduction reached, not the fix for the observed symptom.
func TestLocallyBuiltErrorKeepsCodeAndMessage(t *testing.T) {
	built := &RPCError{Code: ErrCodeMethodNotFound, Message: "method not found: session/fork", Data: map[string]any{}}
	if text := built.Error(); strings.Contains(text, "map[]") {
		t.Fatalf("an empty data payload was rendered as text: %q", text)
	}
	empty := &RPCError{Code: ErrCodeInternal}
	text := empty.Error()
	if !strings.Contains(text, "-32603") || !strings.Contains(text, ErrTextInternal) {
		t.Fatalf("an error with no message lost its code or text: %q", text)
	}
}

// TestNonEmptyErrorPayloadIsRenderedForEveryShape checks the payload rule across
// the JSON shapes a peer can actually send, so "empty" is decided by content and
// not by Go type.
func TestNonEmptyErrorPayloadIsRenderedForEveryShape(t *testing.T) {
	for _, tc := range []struct {
		name     string
		data     any
		wantText bool
	}{
		{name: "nil", data: nil, wantText: false},
		{name: "empty object", data: map[string]any{}, wantText: false},
		{name: "empty list", data: []any{}, wantText: false},
		{name: "blank string", data: "  ", wantText: false},
		{name: "object with a signal", data: map[string]any{"kind": "rate_limited"}, wantText: true},
		{name: "string", data: "quota exceeded", wantText: true},
		{name: "number", data: float64(429), wantText: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rebuilt := rpcErrorFromWire(&jsonRPCError{Code: ErrCodeInternal, Message: "Internal error", Data: tc.data})
			text := rebuilt.Error()
			if strings.Contains(text, "map[]") {
				t.Fatalf("rendered an empty payload: %q", text)
			}
			if got := strings.Contains(text, "("); got != tc.wantText {
				t.Fatalf("payload rendered = %v, want %v: %q", got, tc.wantText, text)
			}
		})
	}
}
