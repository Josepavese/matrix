package zedacp

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestAuthMethodIdentifierReadsBothProtocolGenerations is the parsing half of the
// migration: version 1 names the field "id", version 2 names it "methodId", and a
// client that reads only one of them silently loses every method of the other.
func TestAuthMethodIdentifierReadsBothProtocolGenerations(t *testing.T) {
	var v1 InitializeResponse
	if err := json.Unmarshal([]byte(`{"protocolVersion": 1, "authMethods": [{"id": "legacy-login", "name": "Login"}]}`), &v1); err != nil {
		t.Fatalf("unmarshal v1 response: %v", err)
	}
	if got := v1.AuthMethods[0].Identifier(); got != "legacy-login" {
		t.Fatalf("v1 identifier = %q, want legacy-login", got)
	}

	var v2 InitializeResponse
	if err := json.Unmarshal([]byte(`{"protocolVersion": 2, "authMethods": [{"methodId": "agent-login", "type": "agent", "name": "Login"}]}`), &v2); err != nil {
		t.Fatalf("unmarshal v2 response: %v", err)
	}
	if got := v2.AuthMethods[0].Identifier(); got != "agent-login" {
		t.Fatalf("v2 identifier = %q, want agent-login", got)
	}

	both := AuthMethod{ID: "legacy", MethodID: "modern"}
	if got := both.Identifier(); got != "modern" {
		t.Fatalf("with both fields present the newer one must win, got %q", got)
	}
}

// TestClientSpeaksTheAgreedGeneration pins the wire method names. Sending
// "authenticate" to a v2 agent, or "auth/login" to a v1 agent, is the failure this
// whole change exists to prevent.
func TestClientSpeaksTheAgreedGeneration(t *testing.T) {
	if got := authenticationMethodName(ProtocolVersionV1); got != "authenticate" {
		t.Fatalf("v1 login method = %q, want authenticate", got)
	}
	if got := logoutMethodName(ProtocolVersionV1); got != "logout" {
		t.Fatalf("v1 logout method = %q, want logout", got)
	}
	if got := authenticationMethodName(ProtocolVersionV2); got != "auth/login" {
		t.Fatalf("v2 login method = %q, want auth/login", got)
	}
	if got := logoutMethodName(ProtocolVersionV2); got != "auth/logout" {
		t.Fatalf("v2 logout method = %q, want auth/logout", got)
	}
	// The zero value is "initialize has not run", which must not read as v2.
	if got := authenticationMethodName(0); got != "authenticate" {
		t.Fatalf("an unnegotiated client must default to the v1 surface, got %q", got)
	}
}

// TestInitializeDoesNotAssumeTheVersionItAskedFor: an agent that omits the field
// is a v1 agent, and must not be spoken to in v2 just because 2 was requested.
func TestInitializeDoesNotAssumeTheVersionItAskedFor(t *testing.T) {
	if got := agreedProtocolVersion(0, MaxSupportedProtocolVersion); got != ProtocolVersionV1 {
		t.Fatalf("an undeclared version must be treated as v1, got %d", got)
	}
	if got := agreedProtocolVersion(1, MaxSupportedProtocolVersion); got != ProtocolVersionV1 {
		t.Fatalf("an agent that answers 1 must be spoken to in v1, got %d", got)
	}
	if got := agreedProtocolVersion(2, MaxSupportedProtocolVersion); got != ProtocolVersionV2 {
		t.Fatalf("an agent that answers 2 must be spoken to in v2, got %d", got)
	}
	if got := agreedProtocolVersion(2, ProtocolVersionV1); got != ProtocolVersionV1 {
		t.Fatalf("the agreed version can never exceed the requested one, got %d", got)
	}
}

// TestVersionRejectionIsRecognisedNarrowly: the retry at v1 is triggered only by a
// version-shaped refusal, never by an unrelated initialize failure.
func TestVersionRejectionIsRecognisedNarrowly(t *testing.T) {
	cases := map[string]bool{
		"unsupported protocol version 2":           true,
		"protocolVersion 2 is not supported":       true,
		"unknown protocol version":                 true,
		"failed to start agent process":            false,
		"protocol error: missing session id":       false,
		"the agent refused the requested protocol": false,
		"authentication required":                  false,
	}
	for message, want := range cases {
		if got := rejectedForProtocolVersion(errors.New(message)); got != want {
			t.Fatalf("rejectedForProtocolVersion(%q) = %v, want %v", message, got, want)
		}
	}
	if rejectedForProtocolVersion(nil) {
		t.Fatal("a nil error is not a version rejection")
	}
}

// TestIsAuthenticationRequiredReadsTheStructuredSignal covers the shapes an agent
// can use for the v2 marker, and the two ways it must stay quiet: an unrelated
// error, and an empty payload.
func TestIsAuthenticationRequiredReadsTheStructuredSignal(t *testing.T) {
	required := []*RPCError{
		{Code: -32000, Message: "nope", Data: map[string]any{"kind": "auth_required"}},
		{Code: -32000, Message: "nope", Data: map[string]any{"error": "AUTH_REQUIRED"}},
		{Code: -32000, Message: "nope", Data: map[string]any{"outer": map[string]any{"code": "auth_required"}}},
		{Code: -32000, Message: "nope", Data: []any{map[string]any{"kind": "auth_required"}}},
		{Code: -32000, Message: "nope", Data: "auth_required"},
	}
	for _, err := range required {
		if !IsAuthenticationRequired(err) {
			t.Fatalf("structured marker not recognised: %+v", err)
		}
	}

	quiet := []error{
		nil,
		errors.New("authentication required"), // text only: v1 agents, handled by the caller's fallback
		&RPCError{Code: -32000, Message: "nope", Data: map[string]any{"kind": "rate_limited"}},
		&RPCError{Code: -32000, Message: "nope"},
	}
	for _, err := range quiet {
		if IsAuthenticationRequired(err) {
			t.Fatalf("false positive on %v", err)
		}
	}
}

// TestAuthSignalWalkIsBounded: Data is decoded from bytes the agent process sends,
// so a deeply nested document must not be able to exhaust the stack.
func TestAuthSignalWalkIsBounded(t *testing.T) {
	var nested any = "auth_required"
	for i := 0; i < 5000; i++ {
		nested = map[string]any{"nested": nested}
	}
	if IsAuthenticationRequired(&RPCError{Data: nested}) {
		t.Fatal("a marker buried past the depth bound must not be walked to")
	}
}
