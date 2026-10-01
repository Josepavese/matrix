package runapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/session"
	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
	runresponse "github.com/Josepavese/matrix/internal/providers/runapi/response"
)

func runWorkspaceStorage(t *testing.T, metas ...workspace.Meta) middleware.Storage {
	t.Helper()
	storage := memstore.New()
	for _, meta := range metas {
		if err := workspace.SaveMeta(storage, meta); err != nil {
			t.Fatalf("SaveMeta(%s): %v", meta.ID, err)
		}
	}
	return storage
}

func postRunRequest(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	server.HandleRuns(w, newJSONRequest(http.MethodPost, RunPathV1, strings.NewReader(body)))
	return w
}

func loadRunTrace(t *testing.T, server *Server, runID string) runtrace.Trace {
	t.Helper()
	trace, found, err := server.runStore.Trace(runID)
	if err != nil || !found {
		t.Fatalf("trace(%s): err=%v found=%v", runID, err, found)
	}
	return trace
}

func findRunEvent(t *testing.T, trace runtrace.Trace, kind string) runtrace.Event {
	t.Helper()
	for _, event := range trace.Events {
		if event.Kind == kind {
			return event
		}
	}
	t.Fatalf("run %s has no %s event: %+v", trace.Run.ID, kind, trace.Events)
	return runtrace.Event{}
}

func waitForTerminalRun(t *testing.T, server *Server, runID string) runtrace.TraceRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		trace, found, err := server.runStore.Trace(runID)
		if err == nil && found && trace.Run.Status != runtrace.StatusRunning {
			return trace.Run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s never reached a terminal state", runID)
	return runtrace.TraceRun{}
}

// Issue 6 regression: the same grant has to accept both entrypoints. A caller
// that names only workspace_id used to be refused with matrix_workspace_not_granted
// because the grant was evaluated on the empty path the caller left out.
func TestWorkspaceGrantAcceptsBothEntrypointsOnTheSameGrant(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	testGit(t, "init", "-q", root)
	testGit(t, "-C", root, "-c", "user.name=Matrix", "-c", "user.email=matrix@example.invalid", "commit", "-q", "--allow-empty", "-m", "seed")
	router := &runTestRouter{}
	server := NewServer(router).WithTraceStorage(runWorkspaceStorage(t, workspace.Meta{ID: "ws-grant", RootPath: root}))
	if _, err := server.workspaceGrants.Register(context.Background(), root, false, time.Hour); err != nil {
		t.Fatalf("register grant: %v", err)
	}
	// Both entrypoints must clear the grant preflight itself, not only the HTTP
	// handler: this is where a workspace_id alone used to be refused.
	if err := server.requireWorkspaceGrant(context.Background(), runRequest{WorkspaceID: "ws-grant", WorkspacePolicy: "require_grant"}); err != nil {
		t.Fatalf("workspace_id alone was refused by the grant preflight: %v", err)
	}
	if err := server.requireWorkspaceGrant(context.Background(), runRequest{WorkspaceID: "ws-grant", WorkspacePath: root, WorkspacePolicy: "require_grant"}); err != nil {
		t.Fatalf("workspace_id with its own path was refused by the grant preflight: %v", err)
	}

	byID := postRunRequest(t, server, `{"channel_id":"ws-grant-id","input":"probe by id","workspace_id":"ws-grant","workspace_policy":"require_grant"}`)
	if byID.Code != http.StatusCreated {
		t.Fatalf("workspace_id alone was refused: %d %s", byID.Code, byID.Body.String())
	}
	if router.lastConversation.WorkspaceID != "ws-grant" || router.lastConversation.WorkspacePath != root {
		t.Fatalf("prompt did not carry the canonical workspace: %+v", router.lastConversation)
	}

	router.lastConversation = middleware.ConversationRequest{}
	byIDAndPath := postRunRequest(t, server, fmt.Sprintf(
		`{"channel_id":"ws-grant-id-path","input":"probe by id and path","workspace_id":"ws-grant","workspace_path":%q,"workspace_policy":"require_grant"}`, root))
	if byIDAndPath.Code != http.StatusCreated {
		t.Fatalf("workspace_id with its own path was refused: %d %s", byIDAndPath.Code, byIDAndPath.Body.String())
	}
	if router.lastConversation.WorkspaceID != "ws-grant" || router.lastConversation.WorkspacePath != root {
		t.Fatalf("prompt did not carry the canonical workspace: %+v", router.lastConversation)
	}
}

// The grant is evaluated on the canonical path of the workspace the id names, so
// an unregistered directory stays refused through both entrypoints.
func TestWorkspaceGrantRefusesAPathOutsideTheGrant(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	foreign := filepath.Join(t.TempDir(), "foreign")
	testGit(t, "init", "-q", root)
	testGit(t, "init", "-q", foreign)
	router := &runTestRouter{}
	server := NewServer(router).WithTraceStorage(runWorkspaceStorage(t, workspace.Meta{ID: "ws-foreign", RootPath: foreign}))
	if _, err := server.workspaceGrants.Register(context.Background(), root, false, time.Hour); err != nil {
		t.Fatalf("register grant: %v", err)
	}

	byPath := postRunRequest(t, server, fmt.Sprintf(`{"channel_id":"ws-foreign-path","input":"must not run","workspace_path":%q,"workspace_policy":"require_grant"}`, foreign))
	byID := postRunRequest(t, server, `{"channel_id":"ws-foreign-id","input":"must not run","workspace_id":"ws-foreign","workspace_policy":"require_grant"}`)
	for name, w := range map[string]*httptest.ResponseRecorder{"path": byPath, "id": byID} {
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s entrypoint was not refused by the grant: %d %s", name, w.Code, w.Body.String())
		}
		var refusal runresponse.Error
		if err := json.Unmarshal(w.Body.Bytes(), &refusal); err != nil || refusal.RunID == "" || refusal.Code != "matrix_workspace_not_granted" {
			t.Fatalf("%s refusal lacks a typed run failure: %v %s", name, err, w.Body.String())
		}
		if trace := loadRunTrace(t, server, refusal.RunID); trace.Run.Status != runtrace.StatusFailed {
			t.Fatalf("%s refusal was not recorded as a failed run: %+v", name, trace.Run)
		}
	}
	if router.lastConversation.Input != "" {
		t.Fatalf("a refused run reached the provider: %q", router.lastConversation.Input)
	}
}

// The ambiguous pair is refused with its own type before a session, an agent
// client or a prompt is touched.
func TestWorkspaceIdentityMismatchIsRefusedTypedBeforeThePrompt(t *testing.T) {
	rootA := filepath.Join(t.TempDir(), "a")
	rootB := filepath.Join(t.TempDir(), "b")
	router := &runTestRouter{}
	server := NewServer(router).WithTraceStorage(runWorkspaceStorage(t,
		workspace.Meta{ID: "ws-a", RootPath: rootA},
		workspace.Meta{ID: "ws-b", RootPath: rootB},
	))

	w := postRunRequest(t, server, fmt.Sprintf(
		`{"channel_id":"ws-mismatch","input":"must not run","workspace_id":"ws-a","workspace_path":%q}`, rootB))
	if w.Code != http.StatusConflict {
		t.Fatalf("mismatch must be a conflict, got %d %s", w.Code, w.Body.String())
	}
	var refusal runresponse.Error
	if err := json.Unmarshal(w.Body.Bytes(), &refusal); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	if refusal.Code != WorkspaceIdentityMismatchCode || refusal.RunID == "" {
		t.Fatalf("mismatch was not refused with its own code: %+v", refusal)
	}
	if refusal.Details["registered_workspace_path"] != rootA || refusal.Details["workspace_path"] != rootB {
		t.Fatalf("refusal must name both sides: %+v", refusal.Details)
	}
	if router.lastConversation.Input != "" {
		t.Fatalf("a mismatched run reached the provider: %q", router.lastConversation.Input)
	}
	trace := loadRunTrace(t, server, refusal.RunID)
	if trace.Run.Status != runtrace.StatusFailed {
		t.Fatalf("mismatch was not recorded as a failed run: %+v", trace.Run)
	}
	if event := findRunEvent(t, trace, "provider.preflight.failed"); event.Metadata["code"] != WorkspaceIdentityMismatchCode {
		t.Fatalf("trace lost the typed refusal: %+v", event.Metadata)
	}
}

// blockingAffinityRouter stops inside the prompt so the test can read what the
// run published while no prompt has been answered yet.
type blockingAffinityRouter struct {
	*runTestRouter
	plan    session.SessionAffinityPlan
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingAffinityRouter) RouteConversation(_ context.Context, req middleware.ConversationRequest) (string, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	r.lastConversation = req
	return "ok", nil
}

func (r *blockingAffinityRouter) PlanSessionAffinity(_, _, _, _ string) (session.SessionAffinityPlan, error) {
	return r.plan, nil
}

// Issue 7 regression: a brand new channel that names only a path must show, in
// the trace and before the prompt, the workspace that path resolved to and the
// remote session the routing decision is about to reuse.
func TestRunPublishesWorkspaceIdentityAndPlannedSessionBeforeThePrompt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "halfpocket")
	router := &blockingAffinityRouter{
		runTestRouter: &runTestRouter{},
		plan: session.SessionAffinityPlan{
			Kind:                "resume-workspace-session",
			LogicalSessionID:    "logical-other-workstream",
			RemoteSessionID:     "ses-other-workstream",
			AgentID:             "opencode",
			WorkspaceID:         "ws-existing",
			WorkspacePath:       root,
			ReusesRemoteSession: true,
		},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	server := NewServer(router).WithTraceStorage(runWorkspaceStorage(t, workspace.Meta{ID: "ws-existing", RootPath: root}))

	accepted := postRunRequest(t, server, fmt.Sprintf(
		`{"channel_id":"model-probe-new","input":"answer only","workspace_path":%q,"execution_mode":"async"}`, root))
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("async probe was not accepted: %d %s", accepted.Code, accepted.Body.String())
	}
	var success runresponse.Success
	if err := json.Unmarshal(accepted.Body.Bytes(), &success); err != nil {
		t.Fatalf("decode accepted run: %v", err)
	}

	select {
	case <-router.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the prompt was never attempted")
	}
	// The router is blocked inside the prompt, so anything already in the trace
	// was published before it.
	trace := loadRunTrace(t, server, success.RunID)
	if trace.Run.Status != runtrace.StatusRunning {
		t.Fatalf("expected a non-terminal run while the prompt is blocked, got %q", trace.Run.Status)
	}
	if trace.Run.WorkspaceID != "ws-existing" {
		t.Fatalf("run record does not carry the canonical workspace id: %+v", trace.Run)
	}
	event := findRunEvent(t, trace, "workspace.identity.resolved")
	for key, want := range map[string]interface{}{
		"workspace_id":                  "ws-existing",
		"workspace_path":                root,
		"workspace_source":              workspace.IdentitySourceWorkspacePath,
		"requested_workspace_id":        "",
		"requested_workspace_path":      root,
		"planned_session_kind":          "resume-workspace-session",
		"planned_logical_session_id":    "logical-other-workstream",
		"planned_remote_session_id":     "ses-other-workstream",
		"planned_reuses_remote_session": true,
	} {
		if got := event.Metadata[key]; got != want {
			t.Fatalf("pre-prompt evidence %s=%v want %v (metadata %+v)", key, got, want, event.Metadata)
		}
	}
	close(router.release)
	if run := waitForTerminalRun(t, server, success.RunID); run.Status != runtrace.StatusCompleted {
		t.Fatalf("probe did not complete: %+v", run)
	}
}

// The explicit isolated-session option is visible before the prompt too: the
// evidence says a new session will be created rather than another workstream's
// remote session reused.
func TestIsolatedSessionPolicyIsVisibleBeforeThePrompt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "probe")
	router := &runTestRouter{}
	server := NewServer(router).WithTraceStorage(runWorkspaceStorage(t, workspace.Meta{ID: "ws-probe", RootPath: root}))

	w := postRunRequest(t, server, fmt.Sprintf(
		`{"channel_id":"probe-isolated","input":"probe","workspace_id":"ws-probe","session_policy":%q}`, middleware.SessionPolicyNewEphemeralDeleteAfterRun))
	if w.Code != http.StatusCreated {
		t.Fatalf("isolated probe was not accepted: %d %s", w.Code, w.Body.String())
	}
	var success runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &success); err != nil {
		t.Fatalf("decode: %v", err)
	}
	event := findRunEvent(t, loadRunTrace(t, server, success.RunID), "workspace.identity.resolved")
	if event.Metadata["planned_session_kind"] != "create-isolated-session" ||
		event.Metadata["planned_new_session"] != true ||
		event.Metadata["planned_reuses_remote_session"] != false ||
		event.Metadata["planned_remote_session_id"] != "" {
		t.Fatalf("isolated session was not published before the prompt: %+v", event.Metadata)
	}
}

// The terminal artifact carries both sides of the workspace contract as one
// block, and names what Matrix did not derive instead of leaving the reader to
// read an absent repository field as an agreement.
func TestRunArtifactCarriesTheRequestedAndTheResolvedWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktree")
	server := NewServer(&runTestRouter{}).WithTraceStorage(runWorkspaceStorage(t, workspace.Meta{ID: "ws-worktree", RootPath: root}))

	w := postRunRequest(t, server, fmt.Sprintf(
		`{"channel_id":"probe-workspace-artifact","input":"probe","workspace_id":"ws-worktree","workspace_path":%q}`, root))
	if w.Code != http.StatusCreated {
		t.Fatalf("probe was not accepted: %d %s", w.Code, w.Body.String())
	}
	var success runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &success); err != nil {
		t.Fatalf("decode: %v", err)
	}

	event := findRunEvent(t, loadRunTrace(t, server, success.RunID), "workspace.identity.resolved")
	requested, ok := event.Metadata["workspace_requested"].(map[string]interface{})
	if !ok {
		t.Fatalf("artifact does not carry the requested workspace: %+v", event.Metadata)
	}
	resolved, ok := event.Metadata["workspace_resolved"].(map[string]interface{})
	if !ok {
		t.Fatalf("artifact does not carry the resolved workspace: %+v", event.Metadata)
	}
	if requested["workspace_id"] != "ws-worktree" || requested["path"] != root {
		t.Fatalf("requested side is not what the caller sent: %+v", requested)
	}
	if resolved["workspace_id"] != "ws-worktree" || resolved["path"] != root {
		t.Fatalf("resolved side is not the canonical identity: %+v", resolved)
	}
	notDerived, ok := event.Metadata["workspace_not_derived"].(map[string]interface{})
	if !ok {
		t.Fatalf("artifact does not declare what it did not derive: %+v", event.Metadata)
	}
	for _, field := range []string{"git_common_dir", "branch", "real_child_cwd"} {
		reason, found := notDerived[field].(string)
		if !found || reason == "" {
			t.Fatalf("%s must be declared with its reason, got %+v", field, notDerived)
		}
	}
}
