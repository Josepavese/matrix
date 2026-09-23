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
