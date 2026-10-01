package agentlaunch

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestResolveEndpointFollowsTheProviderPolicyContractNotTheAgentName pins the
// dispatch rule for launch policy: which contract governs a launch is the
// endpoint's own declaration, and the agent's name decides nothing.
//
// The same name appears on both sides of every pair below, so "governed because
// it is codex" and "refused because it is codex" cannot be what makes these pass.
func TestResolveEndpointFollowsTheProviderPolicyContractNotTheAgentName(t *testing.T) {
	governedArgs := []string{
		"wrapper.js",
		"-c", "sandbox_mode=\"danger-full-access\"",
		"-c", "approval_policy=\"never\"",
	}

	t.Run("a declaring endpoint is governed whatever the agent is called", func(t *testing.T) {
		resolved, err := ResolveEndpoint("mimo", middleware.ProtocolEndpoint{
			Args: governedArgs,
			Env:  []string{CodexPolicyContractEnv + "=" + CodexPolicyContractV1},
		})
		if err != nil {
			t.Fatal(err)
		}
		if resolved.Metadata["verified"] != true || resolved.Metadata["trusted_terminal"] != true {
			t.Fatalf("an endpoint that publishes the contract must be governed, got %#v", resolved.Metadata)
		}
		if got := strings.Join(resolved.Endpoint.Args, "\x00"); got != "wrapper.js" {
			t.Fatalf("governed policy args reached provider argv: %q", got)
		}
	})

	t.Run("a codex-named endpoint that declares nothing is not governed by name", func(t *testing.T) {
		// model_reasoning_effort is a launch config key any provider declares
		// for itself (MATRIX_LAUNCH_CONFIG_KEYS), so receiving it is not a claim
		// on the codex launch-policy contract and must not be refused as one.
		resolved, err := ResolveEndpoint("codex", middleware.ProtocolEndpoint{
			Args: []string{"wrapper.js", "-c", "model_reasoning_effort=\"xhigh\""},
		})
		if err != nil {
			t.Fatalf("a declared launch config key must not claim a contract this endpoint never published: %v", err)
		}
		if got := strings.Join(resolved.Endpoint.Args, "\x00"); got != "wrapper.js\x00-c\x00model_reasoning_effort=\"xhigh\"" {
			t.Fatalf("unanchored launch args changed: %q", got)
		}
		if resolved.Metadata != nil {
			t.Fatalf("an undeclared endpoint must carry no policy evidence, got %#v", resolved.Metadata)
		}
	})

	t.Run("a sandbox request without a declared contract is refused whatever the agent is called", func(t *testing.T) {
		resolved, err := ResolveEndpoint("mimo", middleware.ProtocolEndpoint{Args: governedArgs})
		if err == nil || !strings.Contains(err.Error(), "matrix install codex") {
			t.Fatalf("expected the reinstall refusal for a governed key, got %v", err)
		}
		if resolved.Metadata["verified"] != false {
			t.Fatalf("expected unverified evidence, got %#v", resolved.Metadata)
		}
	})
}
