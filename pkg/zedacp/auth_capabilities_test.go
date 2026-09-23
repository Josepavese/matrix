package zedacp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestAuthMethodCarriesTheTerminalLaunchConfiguration pins what a terminal
// method is: a v2 discriminator plus the args and env the client has to append
// to its own configured launch. Dropping any of the three makes Matrix either
// unable to recognize the method or unable to run it correctly.
func TestAuthMethodCarriesTheTerminalLaunchConfiguration(t *testing.T) {
	var resp InitializeResponse
	payload := `{"protocolVersion": 2, "authMethods": [
		{"methodId":"terminal-login","type":"terminal","name":"Log in from the terminal",
		 "args":["--login"],"env":[{"name":"ACP_INTERACTIVE_LOGIN","value":"1"}]},
		{"methodId":"acme","type":"_acme_sso","name":"Acme SSO"},
		{"id":"legacy","name":"Legacy login"}
	]}`
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal v2 authMethods: %v", err)
	}
	if len(resp.AuthMethods) != 3 {
		t.Fatalf("expected three methods, got %+v", resp.AuthMethods)
	}

	terminal := resp.AuthMethods[0]
	if terminal.Identifier() != "terminal-login" || terminal.MethodType() != AuthMethodTypeTerminal {
		t.Fatalf("terminal method misread: %+v", terminal)
	}
	if len(terminal.Args) != 1 || terminal.Args[0] != "--login" {
		t.Fatalf("terminal args lost: %+v", terminal.Args)
	}
	if len(terminal.Env) != 1 || terminal.Env[0].Name != "ACP_INTERACTIVE_LOGIN" || terminal.Env[0].Value != "1" {
		t.Fatalf("terminal env lost: %+v", terminal.Env)
	}

	if !resp.AuthMethods[1].IsCustom() || resp.AuthMethods[1].MethodType() != "_acme_sso" {
		t.Fatalf("custom type must stay recognizable: %+v", resp.AuthMethods[1])
	}
	// A v1 entry has no discriminator at all, which is how an agent-handled
	// login is described in the previous generation.
	if resp.AuthMethods[2].MethodType() != "" || resp.AuthMethods[2].IsCustom() {
		t.Fatalf("a method without a type is not custom: %+v", resp.AuthMethods[2])
	}
}

// TestClientCapabilitiesAdvertiseTerminalAuthenticationAsAnEmptyObject pins the
// exact wire shape an agent looks for: capabilities.auth.terminal, present and
// empty, and absent entirely when Matrix has not opted in.
func TestClientCapabilitiesAdvertiseTerminalAuthenticationAsAnEmptyObject(t *testing.T) {
	enabled, err := json.Marshal(ClientCapabilities{Auth: &AuthCapabilities{Terminal: &TerminalAuthCapabilities{}}})
	if err != nil {
		t.Fatalf("marshal enabled capabilities: %v", err)
	}
	if !strings.Contains(string(enabled), `"auth":{"terminal":{}}`) {
		t.Fatalf("terminal authentication must serialize as an empty auth.terminal object: %s", enabled)
	}

	disabled, err := json.Marshal(ClientCapabilities{})
	if err != nil {
		t.Fatalf("marshal disabled capabilities: %v", err)
	}
	if strings.Contains(string(disabled), "auth") || strings.Contains(string(disabled), "terminal") {
		t.Fatalf("an absent capability must not appear on the wire: %s", disabled)
	}
}

// authRequiredTransport answers every request with ACP v2's structured
// auth_required error.
type authRequiredTransport struct {
	responses chan []byte
}

func newAuthRequiredTransport() *authRequiredTransport {
	return &authRequiredTransport{responses: make(chan []byte, 4)}
}

func (t *authRequiredTransport) Send(_ context.Context, message []byte) error {
	var req jsonRPCRequest
	if err := json.Unmarshal(message, &req); err != nil {
		return err
	}
	payload, err := json.Marshal(jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      cloneRawMessage(req.ID),
		Error: &jsonRPCError{
			Code:    -32000,
			Message: "authentication required",
			Data:    map[string]any{"kind": "auth_required"},
		},
	})
	if err != nil {
		return err
	}
	t.responses <- payload
	return nil
}

func (t *authRequiredTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case message := <-t.responses:
		return message, nil
	}
}

func (t *authRequiredTransport) Close() error { return nil }

// TestStructuredAuthRequiredSurvivesTheClientError is the half of the migration
// that lives in the client's own error path: the marker ACP v2 puts in the error
// data has to reach the caller as data, because a caller that only sees text
// cannot tell a gated request from any other failure.
func TestStructuredAuthRequiredSurvivesTheClientError(t *testing.T) {
	client := NewClient(context.Background(), newAuthRequiredTransport())
	defer client.Close()

	_, err := client.NewSession(context.Background(), NewSessionRequest{Cwd: "/workspace"})
	if err == nil {
		t.Fatal("a gated request must fail")
	}
	if !IsAuthenticationRequired(err) {
		t.Fatalf("the structured marker did not survive the client error path: %v", err)
	}
	if !strings.Contains(err.Error(), "authentication required") {
		t.Fatalf("the agent's message must stay readable: %v", err)
	}
}
