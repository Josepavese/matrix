package agentcfg

import (
	"strings"
	"testing"
)

func TestNormalizeEndpointPreservesEnvironmentIsolation(t *testing.T) {
	endpoint := NormalizeEndpoint(Config{Kind: "acp", Transport: "stdio", Command: "codex-acp", EnvIsolation: true})
	if !endpoint.EnvIsolation {
		t.Fatal("normalized endpoint lost env isolation launch policy")
	}
}

func TestNormalizeEndpointPreservesA2ATenant(t *testing.T) {
	endpoint := NormalizeEndpoint(Config{Kind: "a2a", Address: "https://agent.example/a2a", Tenant: "project-7"})
	if endpoint.Tenant != "project-7" {
		t.Fatalf("normalized endpoint lost A2A tenant: %#v", endpoint)
	}
}

func TestParseHeadersAndNames(t *testing.T) {
	headers, err := ParseHeaders([]string{"Authorization=Bearer secret", "X-Tenant = project-7"})
	if err != nil {
		t.Fatalf("ParseHeaders: %v", err)
	}
	if headers["Authorization"] != "Bearer secret" || headers["X-Tenant"] != "project-7" {
		t.Fatalf("unexpected headers: %#v", headers)
	}
	names := HeaderNames(headers)
	if len(names) != 2 || names[0] != "Authorization" || names[1] != "X-Tenant" {
		t.Fatalf("unexpected header names: %#v", names)
	}
	if _, err := ParseHeaders([]string{"invalid"}); err == nil {
		t.Fatal("expected malformed header to fail closed")
	}
}

func TestLoadEntryRejectsRetiredProtocolField(t *testing.T) {
	store := &memStorage{data: map[string][]byte{
		Key("agent"): []byte(`{"config":{"protocol":"acp","command":"agent"},"override":{}}`),
	}}
	_, err := LoadEntry(store, "agent")
	if err == nil || !strings.Contains(err.Error(), `use "kind"`) {
		t.Fatalf("expected actionable ZERO-LEGACY rejection, got %v", err)
	}
}
