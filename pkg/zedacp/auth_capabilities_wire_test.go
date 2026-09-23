package zedacp

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestInitializeRequestUsesTheParameterNamesOfTheGenerationItRequests: ACP v2
// renamed clientCapabilities to capabilities and clientInfo to info. Sending the v1
// names to a v2 agent hides every advertised capability, including the terminal auth
// surface this client now supports, so the names follow the requested version.
func TestInitializeRequestUsesTheParameterNamesOfTheGenerationItRequests(t *testing.T) {
	req := InitializeRequest{
		ProtocolVersion:    ProtocolVersionV2,
		ClientInfo:         map[string]interface{}{"name": "matrix"},
		ClientCapabilities: &ClientCapabilities{Auth: &AuthCapabilities{Terminal: &TerminalAuthCapabilities{}}},
	}

	v2, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal v2 request: %v", err)
	}
	body := string(v2)
	for _, want := range []string{`"capabilities"`, `"info"`, `"auth"`, `"terminal"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("v2 request is missing %s: %s", want, body)
		}
	}
	for _, unwanted := range []string{`"clientCapabilities"`, `"clientInfo"`} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("v2 request still carries the v1 name %s: %s", unwanted, body)
		}
	}

	req.ProtocolVersion = ProtocolVersionV1
	v1, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal v1 request: %v", err)
	}
	body = string(v1)
	for _, want := range []string{`"clientCapabilities"`, `"clientInfo"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("v1 request is missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, `"capabilities"`) || strings.Contains(body, `"info"`) {
		t.Fatalf("v1 request must not carry the v2 names: %s", body)
	}
}

// TestInitializeResponseReadsTheCapabilityNamesOfBothGenerations: a v2 peer answers
// with capabilities and info, a v1 peer with agentCapabilities and agentInfo. Reading
// only the v1 names left every v2 capability empty, so Matrix believed a spec-shaped
// agent supported nothing at all - including the terminal authentication surface.
func TestInitializeResponseReadsTheCapabilityNamesOfBothGenerations(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "v2 names",
			body: `{"protocolVersion":2,"capabilities":{"terminal":true},"info":{"name":"peer-v2"},` +
				`"authMethods":[{"methodId":"terminal-login","type":"terminal"}]}`,
		},
		{
			name: "v1 names",
			body: `{"protocolVersion":1,"agentCapabilities":{"terminal":true},"agentInfo":{"name":"peer-v1"},` +
				`"authMethods":[{"id":"terminal-login","type":"terminal"}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp InitializeResponse
			if err := json.Unmarshal([]byte(tc.body), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(resp.Capabilities) == 0 {
				t.Fatalf("capabilities were dropped: %+v", resp)
			}
			if resp.Capabilities["terminal"] != true {
				t.Fatalf("capabilities = %+v, want terminal true", resp.Capabilities)
			}
			if len(resp.AgentInfo) == 0 {
				t.Fatalf("agent info was dropped: %+v", resp)
			}
			if len(resp.AuthMethods) != 1 || resp.AuthMethods[0].Identifier() != "terminal-login" {
				t.Fatalf("auth methods = %+v", resp.AuthMethods)
			}
		})
	}

	// A peer that sends both spellings is answered by the generation it declared.
	var both InitializeResponse
	if err := json.Unmarshal([]byte(`{"protocolVersion":2,"capabilities":{"terminal":true},`+
		`"agentCapabilities":{"terminal":false}}`), &both); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if both.Capabilities["terminal"] != true {
		t.Fatalf("the v2 spelling must win: %+v", both.Capabilities)
	}
}
