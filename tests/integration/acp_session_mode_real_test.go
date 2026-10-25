package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/agents"
)

// ----------------------------------------------------------------------------
// Required ACP session mode against the real OpenCode provider
//
// This is the provider-shape half of the neutral session mode contract. The unit
// tests next door prove the adapter's decisions against a fake client; this test
// proves the same decisions against the shape OpenCode 1.18.31 actually serves:
// a custom primary agent declared through OPENCODE_CONFIG_CONTENT appears as a
// session mode option whose tool set is disabled, and Matrix applies it to a
// created session and confirms it from the provider's own set_config_option
// response before the session is usable.
//
// No model call happens: the run only materializes a session, so the test needs
// no credential and cannot depend on a model's answer.
// ----------------------------------------------------------------------------

const (
	opencodeSessionModeOptIn  = "MATRIX_OPENCODE_ACP_STDIO"
	opencodeSessionModeAgent  = "json-planning"
	opencodeToolsFreeEnvProof = "Tool bash is disabled for agent " + opencodeSessionModeAgent
)

// opencodeToolsFreeConfig is the role-scoped declaration the consumer would put
// on the agent endpoint: a primary agent with every native tool disabled. The
// mode identifier is the agent id, which is what OpenCode turns into a session
// mode option.
func opencodeToolsFreeConfig() string {
	tools := map[string]bool{}
	for _, name := range []string{
		"bash", "edit", "read", "write", "glob", "grep", "patch", "webfetch",
		"todowrite", "todoread", "task", "skill", "question", "list",
	} {
		tools[name] = false
	}
	payload, err := json.Marshal(map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"agent": map[string]interface{}{
			opencodeSessionModeAgent: map[string]interface{}{
				"mode":        "primary",
				"description": "JSON planning without tools",
				"tools":       tools,
			},
		},
	})
	if err != nil {
		panic("static OpenCode agent declaration cannot fail to encode: " + err.Error())
	}
	return string(payload)
}

// opencodeSessionModeEnv isolates every path OpenCode writes to under the test's
// temporary directory. The provider is launched by Matrix, so this is also the
// environment Matrix hands its child.
func opencodeSessionModeEnvironment(t *testing.T, base, requiredMode string) []string {
	t.Helper()
	for _, name := range []string{"home", "data", "config", "cache", "state", "npm-cache"} {
		if err := os.MkdirAll(filepath.Join(base, name), 0o755); err != nil {
			t.Fatalf("create isolated %s: %v", name, err)
		}
	}
	return []string{
		"HOME=" + filepath.Join(base, "home"),
		"XDG_DATA_HOME=" + filepath.Join(base, "data"),
		"XDG_CONFIG_HOME=" + filepath.Join(base, "config"),
		"XDG_CACHE_HOME=" + filepath.Join(base, "cache"),
		"XDG_STATE_HOME=" + filepath.Join(base, "state"),
		"OPENCODE_DISABLE_AUTOUPDATE=1",
		"npm_config_cache=" + filepath.Join(base, "npm-cache"),
		"OPENCODE_CONFIG_CONTENT=" + opencodeToolsFreeConfig(),
		agentlaunch.ACPSessionModeEnv + "=" + requiredMode,
	}
}

func materializeOpenCodeSession(t *testing.T, opencodePath, workspace string, env []string) (middleware.RemoteSessionInfo, middleware.ConversationMetadata, error) {
	t.Helper()
	router := agents.NewRouter(&realAgentResolver{
		protocol: "stdio",
		bin:      opencodePath,
		args:     []string{"--pure", "acp"},
		env:      env,
	})
	t.Cleanup(router.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return router.MaterializeAgentSession(ctx, "opencode", middleware.SessionMaterializeRequest{WorkspacePath: workspace})
}

func sessionModeEvidenceOf(t *testing.T, metadata middleware.ConversationMetadata) map[string]interface{} {
	t.Helper()
	raw, ok := metadata.Meta["session_mode"]
	if !ok {
		t.Fatalf("materialized session carries no session mode evidence: %#v", metadata.Meta)
	}
	evidence, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("session mode evidence is not a map: %#v", raw)
	}
	return evidence
}

// TestOpenCodeRequiredSessionModeAppliedAndVerifiedForToolsFreeAgent proves the
// consumer's requirement end to end, without a model call: Matrix applies the
// custom tools-free agent as a required ACP session mode and records the
// provider's confirmation, and a mode the provider does not expose fails the
// session before any prompt could be sent.
func TestOpenCodeRequiredSessionModeAppliedAndVerifiedForToolsFreeAgent(t *testing.T) {
	requireSmokeTest(t)
	if os.Getenv(opencodeSessionModeOptIn) != "1" {
		t.Skip("Set MATRIX_OPENCODE_ACP_STDIO=1 to run the real OpenCode required session mode test.")
	}
	opencodePath := lookPath(t, "opencode")
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	t.Run("required mode is applied and confirmed", func(t *testing.T) {
		env := opencodeSessionModeEnvironment(t, base, opencodeSessionModeAgent)
		// The role's agent is tools-free: the provider itself says the tool is
		// disabled for exactly this agent id, so a tool call cannot be reached by
		// a prompt suggestion or by the model's own initiative.
		// A disabled tool is reported as a refused debug run, so the exit status
		// carries no verdict here: the provider's own sentence does.
		proof := exec.CommandContext(context.Background(), opencodePath, "debug", "agent", opencodeSessionModeAgent, "--tool", "bash", "--params", `{"command":"echo hi"}`)
		proof.Env = append(os.Environ(), env...)
		output, _ := proof.CombinedOutput()
		if !strings.Contains(string(output), opencodeToolsFreeEnvProof) {
			t.Fatalf("the custom agent is not tools-free: %q", strings.TrimSpace(string(output)))
		}

		info, metadata, err := materializeOpenCodeSession(t, opencodePath, workspace, env)
		if err != nil {
			t.Fatalf("Matrix did not apply and verify the required session mode: %v", err)
		}
		if strings.TrimSpace(info.RemoteSessionID) == "" {
			t.Fatalf("materialized session has no remote id: %#v", info)
		}
		evidence := sessionModeEvidenceOf(t, metadata)
		for key, want := range map[string]interface{}{
			"configured_mode": opencodeSessionModeAgent,
			"effective_mode":  opencodeSessionModeAgent,
			"config_id":       "mode",
			"evidence_source": "session/set_config_option",
			"verification":    middleware.ModelVerificationConfirmed,
			"enforcement":     "required",
		} {
			if evidence[key] != want {
				t.Fatalf("session mode evidence %s=%v want %v (%#v)", key, evidence[key], want, evidence)
			}
		}
	})

	t.Run("an unavailable mode fails the session before the prompt", func(t *testing.T) {
		const missing = "matrix-missing-planning-mode"
		env := opencodeSessionModeEnvironment(t, base, missing)
		_, metadata, err := materializeOpenCodeSession(t, opencodePath, workspace, env)
		if err == nil {
			t.Fatalf("Matrix accepted an unavailable required session mode: %#v", metadata)
		}
		if !strings.Contains(err.Error(), `"`+missing+`"`) {
			t.Fatalf("failure does not name the required mode %q: %v", missing, err)
		}
		if !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("failure does not state the mode is unavailable: %v", err)
		}
	})
}
