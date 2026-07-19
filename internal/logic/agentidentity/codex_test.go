package agentidentity

import (
	"strings"
	"testing"
)

func TestValidatePublicAgentIDRejectsProviderIdentifier(t *testing.T) {
	err := ValidatePublicAgentID("codex-acp")
	if err == nil || !strings.Contains(err.Error(), `use "codex"`) {
		t.Fatalf("expected canonical Matrix agent ID error, got %v", err)
	}
}

func TestValidateRuntimeDefinitionRejectsDeprecatedProvider(t *testing.T) {
	err := ValidateRuntimeDefinition("codex", "npx", []string{"-y", "@zed-industries/codex-acp@0.16.0"})
	if err == nil || !strings.Contains(err.Error(), "matrix install codex") {
		t.Fatalf("expected migration error, got %v", err)
	}
}

func TestValidateRuntimeDefinitionAcceptsCanonicalProvider(t *testing.T) {
	err := ValidateRuntimeDefinition("codex", "npx", []string{"-y", "@agentclientprotocol/codex-acp@1.1.4"})
	if err != nil {
		t.Fatalf("unexpected canonical provider error: %v", err)
	}
}
