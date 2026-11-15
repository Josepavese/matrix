package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/agents"
	execprov "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// ----------------------------------------------------------------------------
// ACP v2 qualification against installed commercial agents
//
// The mock peer tests prove the adapter's wire contract. They cannot prove that
// an actual vendor agent accepts Matrix's version 2 initialize, creates a
// session, runs a turn, resumes that session and closes it. This opt-in test
// does exactly that against the endpoints the operator names, so a release can
// state which agent versions were qualified instead of inferring it from a
// protocol document.
//
// It is not part of the default suite because it launches external programs, may
// need a logged-in account and may spend model tokens. Run it explicitly:
//
//	MATRIX_ACP_V2_REAL=1 \
//	MATRIX_ACP_V2_CODEX_ENTRY=/path/to/codex-acp/dist/index.js \
//	MATRIX_ACP_V2_CODEX_PROMPT='Reply with exactly: MATRIX-ACP-V2-OK' \
//	go test ./tests/integration -run TestACPv2CommercialAgentQualification -v
//
// The Claude variables follow the same shape. Leaving the PROMPT variable empty
// qualifies initialize, session/new, capability reporting and close without
// spending a model call; use it for an agent whose local login is unavailable.
// MATRIX_ACP_V2_ENV_FILE names an optional JSON file with an "env" object; its
// entries are passed to the agent process. It lets an operator qualify a peer
// against the same gateway the interactive CLI uses without copying secrets
// into the test command line or logs.
// ----------------------------------------------------------------------------

type acpV2QualificationResolver struct {
	command string
	args    []string
	env     []string
	prompt  middleware.SystemPrompt
}

func (r acpV2QualificationResolver) GetAgentEndpoint(string) (middleware.ProtocolEndpoint, error) {
	return middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   r.command,
		Args:      append([]string{}, r.args...),
		Env:       append([]string{}, r.env...),
	}, nil
}

func (r acpV2QualificationResolver) GetAgentSystemPrompt(string) (middleware.SystemPrompt, error) {
	return r.prompt, nil
}

type acpV2QualificationSpec struct {
	name      string
	agentID   string
	entryEnv  string
	promptEnv string
}

func TestACPv2CommercialAgentQualification(t *testing.T) {
	if os.Getenv("MATRIX_ACP_V2_REAL") != "1" {
		t.Skip("set MATRIX_ACP_V2_REAL=1 to qualify real ACP v2 agents")
	}
	specs := []acpV2QualificationSpec{
		{name: "claude", agentID: "claude-qualification", entryEnv: "MATRIX_ACP_V2_CLAUDE_ENTRY", promptEnv: "MATRIX_ACP_V2_CLAUDE_PROMPT"},
		{name: "codex", agentID: "codex-qualification", entryEnv: "MATRIX_ACP_V2_CODEX_ENTRY", promptEnv: "MATRIX_ACP_V2_CODEX_PROMPT"},
	}
	for _, spec := range specs {
		t.Run(spec.name, func(t *testing.T) {
			entry := os.Getenv(spec.entryEnv)
			if entry == "" {
				t.Skipf("%s is not set", spec.entryEnv)
			}
			node := os.Getenv("MATRIX_ACP_V2_NODE")
			if node == "" {
				node, _ = exec.LookPath("node")
			}
			if node == "" {
				t.Fatal("node was not found; set MATRIX_ACP_V2_NODE")
			}
			agentEnv, err := qualificationEnvironment(os.Getenv("MATRIX_ACP_V2_ENV_FILE"))
			if err != nil {
				t.Fatalf("qualification environment: %v", err)
			}
			resolver := acpV2QualificationResolver{
				command: node,
				args:    []string{entry},
				env:     agentEnv,
				prompt:  qualificationSystemPrompt(),
			}
			workspace := t.TempDir()
			router := agents.NewRouter(resolver)
			router.SetTrustMode(func() bool { return true })
			router.SetProcess(execprov.NewProvider())
			router.SetFS(osfs.NewFSProvider(), workspace)
			defer router.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			caps, err := router.AgentCapabilities(ctx, spec.agentID)
			if err != nil {
				t.Fatalf("capabilities: %v", err)
			}
			if caps.ProtocolVersion != "2" {
				t.Fatalf("negotiated protocol version = %q, want 2 (capabilities: %+v)", caps.ProtocolVersion, caps)
			}
			if caps.Operations["session/prompt"].Stability != "draft" {
				t.Fatalf("v2 capability must be reported as draft: %#v", caps.Operations["session/prompt"])
			}
			if _, ok := caps.Operations["auth/login"]; !ok {
				t.Fatalf("v2 capability report is missing auth/login: %#v", caps.Operations)
			}

			materialized, _, err := router.MaterializeAgentSession(ctx, spec.agentID, middleware.SessionMaterializeRequest{
				LogicalSessionID: "qualification-" + spec.name,
				WorkspacePath:    workspace,
			})
			if err != nil {
				t.Fatalf("session/new: %v", err)
			}
			if materialized.RemoteSessionID == "" {
				t.Fatal("session/new returned an empty remote id")
			}
			if !materialized.CanResume {
				t.Fatalf("a v2 session must be resumable: %+v", materialized)
			}

			prompt := strings.TrimSpace(os.Getenv(spec.promptEnv))
			if prompt == "" {
				if err := router.CloseAgentSessionForWorkspace(ctx, spec.agentID, materialized.RemoteSessionID, workspace); err != nil {
					t.Fatalf("session/close: %v", err)
				}
				return
			}

			output, remoteID, _, metadata, err := router.Route(ctx, middleware.RouteRequest{
				AgentID:          spec.agentID,
				LogicalSessionID: "qualification-turn-" + spec.name,
				WorkspacePath:    workspace,
				Message:          prompt,
			})
			if err != nil {
				t.Fatalf("session/prompt: %v", err)
			}
			if strings.TrimSpace(output) == "" {
				t.Fatal("session/prompt returned an empty answer")
			}
			if remoteID == "" {
				t.Fatal("session/prompt returned an empty remote id")
			}
			if got, _ := metadata.Meta["acp_stop_reason"].(string); got == "" {
				t.Fatalf("turn metadata has no terminal stop reason: %#v", metadata.Meta)
			}

			resumed, err := router.AttachAgentSessionForWorkspace(ctx, spec.agentID, remoteID, workspace)
			if err != nil {
				t.Fatalf("session/resume: %v", err)
			}
			if resumed.VerificationMethod != "session/resume" && resumed.VerificationMethod != "session/load" {
				t.Fatalf("resume verification = %q, want session/resume or session/load", resumed.VerificationMethod)
			}
			if err := router.CloseAgentSessionForWorkspace(ctx, spec.agentID, remoteID, workspace); err != nil {
				t.Fatalf("session/close: %v", err)
			}
			if os.Getenv("MATRIX_ACP_V2_CANCEL") == "1" {
				cancelMaterialized, _, err := router.MaterializeAgentSession(ctx, spec.agentID, middleware.SessionMaterializeRequest{
					LogicalSessionID: "qualification-cancel-" + spec.name,
					WorkspacePath:    workspace,
				})
				if err != nil {
					t.Fatalf("session/new for cancellation: %v", err)
				}
				cancelCtx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
				cancelDone := make(chan cancelTurnResult, 1)
				go func() {
					_, _, _, metadata, err := router.Route(cancelCtx, middleware.RouteRequest{
						AgentID:          spec.agentID,
						AgentSessionID:   cancelMaterialized.RemoteSessionID,
						LogicalSessionID: "qualification-cancel-turn-" + spec.name,
						WorkspacePath:    workspace,
						Message:          "Count slowly from 1 to 100, one number per line with a short comment after each number. Do not stop early.",
					})
					cancelDone <- cancelTurnResult{metadata: metadata, err: err}
				}()
				time.Sleep(2 * time.Second)
				if err := router.CancelAgentSessionForWorkspace(cancelCtx, spec.agentID, cancelMaterialized.RemoteSessionID, workspace); err != nil {
					cancelTimeout()
					t.Fatalf("session/cancel: %v", err)
				}
				select {
				case result := <-cancelDone:
					if result.err != nil {
						t.Fatalf("cancelled turn: %v", result.err)
					}
					if got, _ := result.metadata.Meta["acp_stop_reason"].(string); got != "cancelled" {
						t.Fatalf("cancelled turn stop reason = %q, want cancelled", got)
					}
				case <-time.After(30 * time.Second):
					cancelTimeout()
					t.Fatal("cancelled turn did not return")
				}
				cancelTimeout()
				if err := router.CloseAgentSessionForWorkspace(context.Background(), spec.agentID, cancelMaterialized.RemoteSessionID, workspace); err != nil {
					t.Fatalf("session/close after cancellation: %v", err)
				}
			}
		})
	}
}

func qualificationSystemPrompt() middleware.SystemPrompt {
	text := strings.TrimSpace(os.Getenv("MATRIX_ACP_V2_SYSTEM_PROMPT"))
	if text == "" {
		return middleware.SystemPrompt{}
	}
	return middleware.SystemPrompt{Text: text, Mode: middleware.SystemPromptModeAppend}
}

// qualificationEnvironment reads the optional operator-provided JSON document
// ({"env": {"NAME": "value"}}) and converts its entries to the KEY=VALUE form
// the resolver passes to the agent process. It never logs a value.
func qualificationEnvironment(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var document struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	env := make([]string, 0, len(document.Env))
	for name, value := range document.Env {
		if strings.TrimSpace(name) == "" {
			continue
		}
		env = append(env, name+"="+value)
	}
	sort.Strings(env)
	return env, nil
}

type cancelTurnResult struct {
	metadata middleware.ConversationMetadata
	err      error
}
