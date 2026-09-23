package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/agents"
	execprov "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// ----------------------------------------------------------------------------
// The ACP v2 prompt lifecycle against a real peer process
//
// The unit tests next door prove the adapter's own wiring. These drive the
// lifecycle over real stdio, against the repository's own mock peer, because the
// contract under test is a wire one:
//
//   - a peer that acknowledges the prompt, streams its answer and reports the
//     terminal idle state late must still deliver the whole answer: the turn
//     ends on the terminal signal, never on the prompt response plus a quiet
//     moment;
//   - a peer that acknowledges insertion and reports nothing else must end the
//     turn within the configured budget, with the early return visible in the
//     result instead of silent;
//   - a version 1 peer keeps the behaviour it always had.
//
// The mock peer's prompt modes and their environment names are a contract with
// cmd/mock-agent/acp_v2.go and are repeated here on purpose: a rename that lands
// on one side only has to fail these tests rather than move both ends silently.
// ----------------------------------------------------------------------------

const (
	mockPromptModeEnv    = "MOCK_AGENT_V2_PROMPT_MODE"
	mockTerminalDelayEnv = "MOCK_AGENT_V2_TERMINAL_DELAY"
	mockPromptModeRich   = "rich"
	mockPromptModeSilent = "silent"
	mockPromptModeLegacy = "legacy"

	// mockUpsertText and mockLateText are what the rich turn replaces the
	// streamed chunk with and appends after the terminal delay.
	mockUpsertText = "v2 prompt accepted (upserted)"
	mockLateText   = " v2 late content"

	// v2TurnBudgetEnv is the operator override the adapter reads.
	v2TurnBudgetEnv = "MATRIX_ACP_V2_TURN_BUDGET"
	// v1MockAnswer is what the version 1 peer answers by default.
	v1MockAnswer = "I am a mock agent responding via stdio."
	// terminalDelay is longer than the version 1 quiet wait, so a turn that
	// still ended on that wait would return before the late content arrived.
	terminalDelay = 400 * time.Millisecond
)

// newLifecycleRouter builds a router over the real peer with the given v2 prompt
// mode. The credential file exists, so the peer is authenticated at startup and
// the gated prompt flow stays out of the way of the lifecycle assertions.
func newLifecycleRouter(t *testing.T, bin, workspace string, v2 bool, peerEnv ...string) *agents.Router {
	t.Helper()
	evidenceDir := t.TempDir()
	credentialPath := filepath.Join(evidenceDir, "terminal-credential.json")
	if err := os.WriteFile(credentialPath, []byte(`{"token":"lifecycle-test"}`), 0o600); err != nil {
		t.Fatalf("write the peer credential: %v", err)
	}
	args := []string{"--cwd", workspace}
	env := []string{mockCredentialEnv + "=" + credentialPath}
	if v2 {
		args = append(args, mockV2Flag)
		env = append(env,
			mockAuthTypeEnv+"="+mockAuthTypeAgent,
			mockLogEnv+"="+filepath.Join(evidenceDir, "peer.jsonl"),
		)
	}
	env = append(env, peerEnv...)
	resolver := &acpV2MockResolver{bin: bin, args: args, env: env}
	router := agents.NewRouter(resolver)
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(execprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), workspace)
	t.Cleanup(router.Close)
	return router
}

func routeLifecyclePrompt(ctx context.Context, t *testing.T, router *agents.Router, workspace, logicalID string) middleware.ConversationMetadata {
	t.Helper()
	output, _, _, metadata, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          mockV2AgentID,
		LogicalSessionID: logicalID,
		WorkspacePath:    workspace,
		Message:          "hello from the v2 lifecycle test",
	})
	if err != nil {
		t.Fatalf("the prompt must complete: %v", err)
	}
	if output == "" {
		t.Fatal("the turn returned no output at all")
	}
	return metadata
}

// TestACPv2TurnReturnsTheWholeAnswerWhenTheTerminalStateArrivesLate is the
// completion proof over real stdio: the answer's last part and the terminal
// state both arrive after the quiet window that used to end the turn.
func TestACPv2TurnReturnsTheWholeAnswerWhenTheTerminalStateArrivesLate(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()
	router := newLifecycleRouter(t, bin, workspace, true,
		mockPromptModeEnv+"="+mockPromptModeRich,
		mockTerminalDelayEnv+"="+terminalDelay.String(),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	output, _, _, metadata, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          mockV2AgentID,
		LogicalSessionID: "acp-v2-lifecycle-late",
		WorkspacePath:    workspace,
		Message:          "hello from the v2 lifecycle test",
	})
	if err != nil {
		t.Fatalf("the turn must end on the peer's terminal state: %v", err)
	}
	if !strings.Contains(output, mockUpsertText) {
		t.Fatalf("the whole-message upsert must replace the streamed chunk, output=%q", output)
	}
	if strings.Count(output, mockUpsertText) != 1 {
		t.Fatalf("an upsert replaces its message's content instead of appending to it, output=%q", output)
	}
	if !strings.Contains(output, mockLateText) {
		t.Fatalf("content that arrived after the quiet window must still be returned, output=%q", output)
	}
	if got := metadata.Meta["acp_turn_completion"]; got != "idle" {
		t.Fatalf("the result must record the terminal state, got %#v", metadata.Meta)
	}
	if got := metadata.Meta["acp_stop_reason"]; got != "end_turn" {
		t.Fatalf("the specification's stop reason must survive the wire, got %#v", got)
	}
}

// TestACPv2TurnWithoutATerminalStateEndsWithinItsBudget is the bound over real
// stdio: neither a peer that says nothing after the acknowledgement nor one that
// streams content but never reports idle may hold the turn open, and both have
// to say so in the result.
func TestACPv2TurnWithoutATerminalStateEndsWithinItsBudget(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	const budget = 300 * time.Millisecond
	t.Setenv(v2TurnBudgetEnv, budget.String())

	for _, tc := range []struct {
		name         string
		mode         string
		wantsContent string
	}{
		{name: "silent peer", mode: mockPromptModeSilent},
		{name: "legacy peer that never reports idle", mode: mockPromptModeLegacy, wantsContent: mockAcceptedText},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			router := newLifecycleRouter(t, bin, workspace, true, mockPromptModeEnv+"="+tc.mode)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			started := time.Now()
			output, _, _, metadata, err := router.Route(ctx, middleware.RouteRequest{
				AgentID:          mockV2AgentID,
				LogicalSessionID: "acp-v2-lifecycle-budget-" + tc.mode,
				WorkspacePath:    workspace,
				Message:          "hello from the v2 lifecycle test",
			})
			elapsed := time.Since(started)
			if err != nil {
				t.Fatalf("a peer that never finishes must bound the turn, not fail it: %v", err)
			}
			if elapsed < budget {
				t.Fatalf("the turn must wait for its budget, returned after %s", elapsed)
			}
			if elapsed > 20*time.Second {
				t.Fatalf("the turn must end within its budget, took %s", elapsed)
			}
			if got := metadata.Meta["acp_turn_completion"]; got != "timeout" {
				t.Fatalf("the early return must be visible in the result, got %#v", metadata.Meta)
			}
			if tc.wantsContent != "" && !strings.Contains(output, tc.wantsContent) {
				t.Fatalf("what the peer did send must still be returned, output=%q", output)
			}
		})
	}
}

// TestACPv2DefaultPromptModeReportsTheTerminalState pins the peer's default: the
// other tests name a mode explicitly, so without this one a mock that quietly
// stopped reporting idle would leave every lifecycle assertion passing for the
// wrong reason. The budget is short so a regression fails here instead of
// waiting out the real one.
func TestACPv2DefaultPromptModeReportsTheTerminalState(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()
	t.Setenv(v2TurnBudgetEnv, "2s")
	router := newLifecycleRouter(t, bin, workspace, true)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	metadata := routeLifecyclePrompt(ctx, t, router, workspace, "acp-v2-lifecycle-default")
	if got := metadata.Meta["acp_turn_completion"]; got != "idle" {
		t.Fatalf("the default v2 prompt mode must report the terminal state, got %#v", metadata.Meta)
	}
}

// TestACPv1TurnKeepsItsOwnLifecycle pins the generation this work must not
// change: the version 1 peer, the version 1 quiet wait, and no version 2
// completion metadata on the result.
func TestACPv1TurnKeepsItsOwnLifecycle(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()
	router := newLifecycleRouter(t, bin, workspace, false)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	output, _, _, metadata, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          mockV2AgentID,
		LogicalSessionID: "acp-v1-lifecycle",
		WorkspacePath:    workspace,
		Message:          "hello from the v1 lifecycle test",
	})
	if err != nil {
		t.Fatalf("the version 1 turn must succeed: %v", err)
	}
	if !strings.Contains(output, v1MockAnswer) {
		t.Fatalf("the version 1 answer must be returned unchanged, output=%q", output)
	}
	for _, key := range []string{"acp_turn_completion", "acp_stop_reason", "acp_turn_budget_ms"} {
		if value, ok := metadata.Meta[key]; ok {
			t.Fatalf("version 1 must not carry the version 2 key %q (got %#v)", key, value)
		}
	}
}
