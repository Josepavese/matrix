package agentmgr

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentidentity"
)

func TestFindAgentResolvesCodexAliasToCanonicalRegistryID(t *testing.T) {
	agents := []AgentManifest{{ID: "codex-acp", Version: "1.1.2"}}

	got, err := findAgent(agents, "codex")
	if err != nil {
		t.Fatalf("findAgent failed: %v", err)
	}
	if got.ID != "codex-acp" {
		t.Fatalf("resolved id = %q", got.ID)
	}
}

func TestFindAgentRejectsCodexProviderIdentifierAsPublicAgentID(t *testing.T) {
	agents := []AgentManifest{{ID: "codex-acp", Version: "1.1.4"}}

	_, err := findAgent(agents, "codex-acp")
	if err == nil || !strings.Contains(err.Error(), `use "codex"`) {
		t.Fatalf("expected canonical Matrix agent ID error, got %v", err)
	}
}

func TestResolveAnyDistributionRejectsDeprecatedCodexPackage(t *testing.T) {
	client := NewRegistryClient(nil, "")
	manifest := &AgentManifest{ID: "codex-acp", Distribution: RegistryDistribution{
		Npx: &NpxDist{Package: agentidentity.DeprecatedCodexPackage + "@0.16.0"},
	}}

	_, err := client.ResolveAnyDistribution(manifest)
	if err == nil || !strings.Contains(err.Error(), "@agentclientprotocol/codex-acp") {
		t.Fatalf("expected canonical replacement error, got %v", err)
	}
}

func TestResolveAnyDistributionAcceptsCanonicalCodexPackage(t *testing.T) {
	client := NewRegistryClient(nil, "")
	manifest := &AgentManifest{ID: "codex-acp", Distribution: RegistryDistribution{
		Npx: &NpxDist{Package: "@agentclientprotocol/codex-acp@1.1.2"},
	}}

	got, err := client.ResolveAnyDistribution(manifest)
	if err != nil {
		t.Fatalf("ResolveAnyDistribution failed: %v", err)
	}
	if got.Command != "npx" || len(got.Args) != 2 || got.Args[1] != "@agentclientprotocol/codex-acp@1.1.2" {
		t.Fatalf("unexpected resolved distribution: %+v", got)
	}
}
