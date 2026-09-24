package agents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// Opt-in provider proof: MATRIX_TEST_DSH_* point at an isolated DSH_HOME
// containing a copied external session. The test never sends a prompt.
func TestRealDSHStrictExternalAttach(t *testing.T) {
	bin := os.Getenv("MATRIX_TEST_DSH_BIN")
	entry := os.Getenv("MATRIX_TEST_DSH_ENTRY")
	home := os.Getenv("MATRIX_TEST_DSH_HOME")
	id := os.Getenv("MATRIX_TEST_DSH_SESSION_ID")
	workspace := os.Getenv("MATRIX_TEST_DSH_WORKSPACE")
	if bin == "" || entry == "" || home == "" || id == "" || workspace == "" {
		t.Skip("set MATRIX_TEST_DSH_BIN, ENTRY, HOME, SESSION_ID, and WORKSPACE for isolated DSH proof")
	}
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		client, err := (&acpConversationFactory{}).NewClient(ctx, middleware.ProtocolEndpoint{
			Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: bin,
			Args: []string{entry, "--profile", "acp"}, Env: []string{"DSH_HOME=" + home},
		}, middleware.ConversationFactoryDeps{AgentID: "dsh", Cwd: workspace})
		if err != nil {
			cancel()
			t.Fatalf("initialize DSH ACP attempt %d: %v", attempt, err)
		}
		attacher, ok := client.(middleware.ConversationSessionAttacher)
		if !ok {
			_ = client.Close()
			cancel()
			t.Fatal("ACP client does not expose strict attach")
		}
		info, err := attacher.AttachExistingRemoteSession(ctx, id, workspace)
		_ = client.Close()
		cancel()
		if err != nil {
			t.Fatalf("attach external DSH session attempt %d: %v", attempt, err)
		}
		if info.RemoteSessionID != id || info.VerificationMethod != "session/resume" {
			t.Fatalf("wrong remote identity or proof on attempt %d: %+v", attempt, info)
		}
	}
}

func TestRealDSHWorktreePrompt(t *testing.T) {
	if os.Getenv("MATRIX_TEST_DSH_WORKTREE") != "1" {
		t.Skip("set MATRIX_TEST_DSH_WORKTREE=1 with an isolated DSH_HOME")
	}
	bin, entry, home := os.Getenv("MATRIX_TEST_DSH_BIN"), os.Getenv("MATRIX_TEST_DSH_ENTRY"), os.Getenv("MATRIX_TEST_DSH_HOME")
	if bin == "" || entry == "" || home == "" {
		t.Fatal("MATRIX_TEST_DSH_BIN, ENTRY and HOME are required")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	linked := filepath.Join(t.TempDir(), "linked")
	for _, args := range [][]string{{"init", "-q", repo},
		{"-C", repo, "-c", "user.name=Matrix", "-c", "user.email=matrix@example.invalid", "commit", "-q", "--allow-empty", "-m", "seed"},
		{"-C", repo, "worktree", "add", "-q", "-b", "dsh-test", linked}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	marker := "synthetic-worktree-lantern-5182"
	if err := os.WriteFile(filepath.Join(linked, "marker.txt"), []byte(marker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := (&acpConversationFactory{}).NewClient(ctx, middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: bin,
		Args: []string{entry, "--profile", "acp"}, Env: []string{"DSH_HOME=" + home}},
		middleware.ConversationFactoryDeps{AgentID: "dsh", Cwd: linked})
	if err != nil {
		t.Fatalf("DSH worktree client: %v", err)
	}
	defer client.Close()
	result, err := client.ExecuteTurn(ctx, middleware.ConversationTurn{AgentID: "dsh", LogicalSessionID: "worktree-synthetic",
		WorkspacePath: linked, Message: "Read marker.txt in this workspace and reply with only its exact content."})
	if err != nil {
		t.Fatalf("DSH worktree prompt: %v", err)
	}
	if !strings.Contains(result.Output, marker) {
		t.Fatalf("DSH worktree was not read: remote_id=%s output_len=%d", result.RemoteSessionID, len(result.Output))
	}
}

func TestRealDSHExistingWorktreeRead(t *testing.T) {
	workspace := os.Getenv("MATRIX_TEST_DSH_EXISTING_WORKTREE")
	if workspace == "" {
		t.Skip("set MATRIX_TEST_DSH_EXISTING_WORKTREE to reproduce a provider worktree refusal")
	}
	bin, entry, home := os.Getenv("MATRIX_TEST_DSH_BIN"), os.Getenv("MATRIX_TEST_DSH_ENTRY"), os.Getenv("MATRIX_TEST_DSH_HOME")
	if bin == "" || entry == "" || home == "" {
		t.Fatal("MATRIX_TEST_DSH_BIN, ENTRY and HOME are required")
	}
	readme, err := os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	firstLine := strings.SplitN(string(readme), "\n", 2)[0]
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := (&acpConversationFactory{}).NewClient(ctx, middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: bin,
		Args: []string{entry, "--profile", "acp"}, Env: []string{"DSH_HOME=" + home}},
		middleware.ConversationFactoryDeps{AgentID: "dsh", Cwd: workspace})
	if err != nil {
		t.Fatalf("DSH existing worktree client: %v", err)
	}
	defer client.Close()
	result, err := client.ExecuteTurn(ctx, middleware.ConversationTurn{AgentID: "dsh", LogicalSessionID: "existing-worktree-read",
		WorkspacePath: workspace, Message: "Read only the first line of README.md in this workspace and reply with that exact line. Do not modify files."})
	if err != nil {
		t.Fatalf("DSH existing worktree prompt: %v", err)
	}
	if !strings.Contains(result.Output, firstLine) {
		t.Fatalf("DSH did not read existing worktree: remote_id=%s output_len=%d", result.RemoteSessionID, len(result.Output))
	}
}

// This opt-in test creates a synthetic session through a direct ACP client,
// outside Matrix's session mirror, then resumes it through Matrix on a new
// provider process. It requires an isolated DSH_HOME with provider credentials.
func TestRealDSHSyntheticContextSurvivesStrictAttach(t *testing.T) {
	if os.Getenv("MATRIX_TEST_DSH_SYNTHETIC") != "1" {
		t.Skip("set MATRIX_TEST_DSH_SYNTHETIC=1 with an isolated DSH_HOME")
	}
	bin := os.Getenv("MATRIX_TEST_DSH_BIN")
	entry := os.Getenv("MATRIX_TEST_DSH_ENTRY")
	home := os.Getenv("MATRIX_TEST_DSH_HOME")
	if bin == "" || entry == "" || home == "" {
		t.Fatal("MATRIX_TEST_DSH_BIN, ENTRY and HOME are required")
	}
	workspace := t.TempDir()
	codeword := "synthetic-matrix-orchid-74291"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: bin,
		Args: []string{entry, "--profile", "acp"}, Env: []string{"DSH_HOME=" + home}}
	transport, err := createTransport(ctx, transportSpec{Protocol: "stdio", Command: bin, Args: endpoint.Args, Env: endpoint.Env})
	if err != nil {
		t.Fatal(err)
	}
	direct := zedacp.NewClient(ctx, transport)
	if _, err := direct.Initialize(ctx, zedacp.InitializeRequest{ProtocolVersion: 1,
		ClientInfo: map[string]interface{}{"name": "external-synthetic-fixture", "version": "1"}}); err != nil {
		_ = direct.Close()
		t.Fatalf("external DSH initialize: %v", err)
	}
	created, err := direct.NewSession(ctx, zedacp.NewSessionRequest{Cwd: workspace})
	if err != nil {
		_ = direct.Close()
		t.Fatalf("external DSH session/new: %v", err)
	}
	_, err = direct.Prompt(ctx, zedacp.PromptRequest{SessionID: created.SessionID,
		Prompt: []zedacp.Content{{Type: "text", Text: "Remember this synthetic codeword for my next question: " + codeword + ". Reply OK."}}}, nil)
	_ = direct.Close()
	if err != nil {
		t.Fatalf("external DSH seed prompt: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		client, err := (&acpConversationFactory{}).NewClient(ctx, endpoint, middleware.ConversationFactoryDeps{AgentID: "dsh", Cwd: workspace})
		if err != nil {
			t.Fatalf("Matrix DSH client attempt %d: %v", attempt, err)
		}
		attacher := client.(middleware.ConversationSessionAttacher)
		receipt, err := attacher.AttachExistingRemoteSession(ctx, created.SessionID, workspace)
		if err != nil || receipt.RemoteSessionID != created.SessionID {
			_ = client.Close()
			t.Fatalf("strict external attach attempt %d: %+v err=%v", attempt, receipt, err)
		}
		result, err := client.ExecuteTurn(ctx, middleware.ConversationTurn{AgentID: "dsh", LogicalSessionID: "synthetic",
			RemoteSessionID: created.SessionID, StrictSession: true, WorkspacePath: workspace,
			Message: "What synthetic codeword did I ask you to remember? Reply with only that codeword."})
		_ = client.Close()
		if err != nil {
			t.Fatalf("strict resumed prompt attempt %d: %v", attempt, err)
		}
		if result.RemoteSessionID != created.SessionID || !strings.Contains(result.Output, codeword) {
			t.Fatalf("external context was not recovered on attempt %d: remote_id=%s output_len=%d", attempt, result.RemoteSessionID, len(result.Output))
		}
	}
}
