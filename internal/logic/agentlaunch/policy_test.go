package agentlaunch

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestResolveEndpointAppliesCodexTrustedPolicyThroughProviderEnv(t *testing.T) {
	resolved, err := ResolveEndpoint("codex", middleware.ProtocolEndpoint{
		Args: []string{"wrapper.js", "-c", "sandbox_mode=\"danger-full-access\"", "-c", "approval_policy=\"never\"", "-c", "model_reasoning_effort=\"xhigh\""},
		Env:  []string{CodexPolicyContractEnv + "=" + CodexPolicyContractV1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(resolved.Endpoint.Args, "\x00"); got != "wrapper.js" {
		t.Fatalf("policy args reached ignored wrapper argv: %q", got)
	}
	if got := envValue(resolved.Endpoint.Env, codexInitialModeEnv); got != "agent-full-access" {
		t.Fatalf("initial mode = %q", got)
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(envValue(resolved.Endpoint.Env, codexConfigEnv)), &config); err != nil {
		t.Fatal(err)
	}
	if config["sandbox_mode"] != "danger-full-access" || config["approval_policy"] != "never" || config["model_reasoning_effort"] != "xhigh" {
		t.Fatalf("unexpected CODEX_CONFIG: %#v", config)
	}
	if resolved.Metadata["verified"] != true || resolved.Metadata["trusted_terminal"] != true {
		t.Fatalf("expected verified trusted policy, got %#v", resolved.Metadata)
	}
	effective, ok := resolved.Metadata["effective"].(map[string]interface{})
	if !ok || effective["sandbox_mode"] != "danger-full-access" || effective["approval_policy"] != "never" {
		t.Fatalf("unexpected effective policy: %#v", resolved.Metadata)
	}
}

func TestResolveEndpointDetectsCodexBypassFlag(t *testing.T) {
	resolved, err := ResolveEndpoint("codex", middleware.ProtocolEndpoint{
		Args: []string{"wrapper.js", "--dangerously-bypass-approvals-and-sandbox"},
		Env:  []string{CodexPolicyContractEnv + "=" + CodexPolicyContractV1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Metadata["bypass_approvals_and_sandbox"] != true || resolved.Metadata["trusted_terminal"] != true {
		t.Fatalf("expected trusted bypass evidence, got %#v", resolved.Metadata)
	}
}

func TestResolveEndpointFailsClosedWithoutInstalledProviderContract(t *testing.T) {
	resolved, err := ResolveEndpoint("codex", middleware.ProtocolEndpoint{
		Args: []string{"wrapper.js", "-c", "sandbox_mode=danger-full-access", "-c", "approval_policy=never"},
	})
	if err == nil || !strings.Contains(err.Error(), "matrix install codex") {
		t.Fatalf("expected reinstall error, got %v", err)
	}
	if resolved.Metadata["verified"] != false {
		t.Fatalf("expected unverified evidence, got %#v", resolved.Metadata)
	}
}

func TestResolveEndpointFailsClosedOnIncompleteCodexMode(t *testing.T) {
	_, err := ResolveEndpoint("codex", middleware.ProtocolEndpoint{
		Args: []string{"wrapper.js", "-c", "approval_policy=never"},
		Env:  []string{CodexPolicyContractEnv + "=" + CodexPolicyContractV1},
	})
	if err == nil || !strings.Contains(err.Error(), "requires both") {
		t.Fatalf("expected incomplete policy error, got %v", err)
	}
}

func TestResolveEndpointLeavesUnknownAndNonCodexArgsUntouched(t *testing.T) {
	endpoint := middleware.ProtocolEndpoint{Args: []string{"acp", "--model", "gpt-5"}}
	resolved, err := ResolveEndpoint("opencode", endpoint, "--extra")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(resolved.Endpoint.Args, "\x00"); got != "acp\x00--model\x00gpt-5\x00--extra" {
		t.Fatalf("unexpected args: %q", got)
	}
	if resolved.Metadata != nil {
		t.Fatalf("unexpected metadata: %#v", resolved.Metadata)
	}
}

func TestResolveEndpointPreservesUnknownCodexConfigArg(t *testing.T) {
	resolved, err := ResolveEndpoint("codex", middleware.ProtocolEndpoint{
		Args: []string{"wrapper.js", "-c", "model=gpt-5", "-c", "model_reasoning_effort=high"},
		Env:  []string{CodexPolicyContractEnv + "=" + CodexPolicyContractV1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(resolved.Endpoint.Args, "\x00"); got != "wrapper.js\x00-c\x00model=gpt-5" {
		t.Fatalf("unknown config arg changed: %q", got)
	}
}
