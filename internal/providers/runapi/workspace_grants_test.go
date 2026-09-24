package runapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/logic/workspacegrant"
	"github.com/Josepavese/matrix/internal/middleware"
)

func testGit(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func TestWorkspaceGrantAPIAndRunPreflight(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	linked := filepath.Join(t.TempDir(), "linked")
	testGit(t, "init", "-q", root)
	testGit(t, "-C", root, "-c", "user.name=Matrix", "-c", "user.email=matrix@example.invalid", "commit", "-q", "--allow-empty", "-m", "seed")
	testGit(t, "-C", root, "worktree", "add", "-q", "-b", "linked", linked)
	router := &runTestRouter{}
	server := NewServer(router).WithAPIKey("secret")
	registerBody := fmt.Sprintf(`{"repository_path":%q,"include_git_worktrees":true}`, root)
	register := newJSONRequest(http.MethodPost, WorkspaceGrantsPathV1, strings.NewReader(registerBody))
	register.Header.Set("X-Matrix-Key", "secret")
	w := httptest.NewRecorder()
	server.HandleWorkspaceGrants(w, register)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	var grant workspacegrant.Grant
	if err := json.Unmarshal(w.Body.Bytes(), &grant); err != nil {
		t.Fatal(err)
	}
	if grant.ID == "" || !grant.IncludeGitWorktrees {
		t.Fatalf("invalid grant: %+v", grant)
	}
	if err := server.requireWorkspaceGrant(context.Background(), runRequest{WorkspacePath: linked, WorkspacePolicy: "require_grant"}); err != nil {
		t.Fatalf("linked worktree refused: %v", err)
	}
	allowedBody := fmt.Sprintf(`{"channel_id":"grant-test-allowed","input":"worktree task","workspace_path":%q,"workspace_policy":"require_grant"}`, linked)
	allowed := newJSONRequest(http.MethodPost, RunPathV1, strings.NewReader(allowedBody))
	allowed.Header.Set("X-Matrix-Key", "secret")
	w = httptest.NewRecorder()
	server.HandleRuns(w, allowed)
	if w.Code != http.StatusCreated || router.lastConversation.Input != "worktree task" {
		t.Fatalf("granted run failed to reach provider: %d %s", w.Code, w.Body.String())
	}
	router.lastConversation = middleware.ConversationRequest{}
	foreign := filepath.Join(t.TempDir(), "foreign")
	testGit(t, "init", "-q", foreign)
	err := server.requireWorkspaceGrant(context.Background(), runRequest{WorkspacePath: foreign, WorkspacePolicy: "require_grant"})
	failure, ok := providerfailure.As(err)
	if !ok || failure.Code != providerfailure.WorkspaceNotGranted {
		t.Fatalf("foreign repository was not refused by Matrix: %v", err)
	}
	runBody := fmt.Sprintf(`{"channel_id":"grant-test","input":"do not deliver","workspace_path":%q,"workspace_policy":"require_grant"}`, foreign)
	run := newJSONRequest(http.MethodPost, RunPathV1, strings.NewReader(runBody))
	run.Header.Set("X-Matrix-Key", "secret")
	w = httptest.NewRecorder()
	server.HandleRuns(w, run)
	if w.Code != http.StatusForbidden || router.lastConversation.Input != "" {
		t.Fatalf("run preflight did not stop before prompt: %d %s", w.Code, w.Body.String())
	}
	var refusal struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &refusal); err != nil || refusal.RunID == "" {
		t.Fatalf("missing refused run ID: %v %s", err, w.Body.String())
	}
	trace, found, err := server.runStore.Trace(refusal.RunID)
	if err != nil || !found || trace.Run.Status != "failed" {
		t.Fatalf("workspace refusal missing from trace: %+v found=%v err=%v", trace, found, err)
	}
	revoke := httptest.NewRequest(http.MethodDelete, WorkspaceGrantsPathV1+"/"+grant.ID, nil)
	revoke.Header.Set("X-Matrix-Key", "secret")
	w = httptest.NewRecorder()
	server.HandleWorkspaceGrants(w, revoke)
	if w.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}
	if err := server.requireWorkspaceGrant(context.Background(), runRequest{WorkspacePath: linked, WorkspacePolicy: "require_grant"}); err == nil {
		t.Fatal("revoked worktree grant still accepted")
	}
}
