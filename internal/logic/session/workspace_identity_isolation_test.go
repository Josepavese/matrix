package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
)

func newWorkspaceSessionManager(t *testing.T, router middleware.AgentRouter, metas ...workspace.Meta) (*Manager, *mockStorage) {
	t.Helper()
	storage := &mockStorage{data: map[string][]byte{}}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("set configured: %v", err)
	}
	for _, meta := range metas {
		if err := workspace.SaveMeta(storage, meta); err != nil {
			t.Fatalf("SaveMeta(%s): %v", meta.ID, err)
		}
	}
	return NewManager(storage, router, newTestWizard(storage), nil), storage
}

func routeWorkspaceProbe(t *testing.T, mgr *Manager, channelID, workspaceID, workspacePath string) error {
	t.Helper()
	_, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:     channelID,
		WorkspaceID:   workspaceID,
		WorkspacePath: workspacePath,
		Input:         "probe",
	})
	return err
}

func activeSessionIDFor(t *testing.T, mgr *Manager, channelID string) string {
	t.Helper()
	state, err := mgr.getChannelState(channelID)
	if err != nil {
		t.Fatalf("getChannelState(%s): %v", channelID, err)
	}
	if strings.TrimSpace(state.ActiveSessionID) == "" {
		t.Fatalf("channel %s has no active session", channelID)
	}
	return state.ActiveSessionID
}

func sessionMetaFor(t *testing.T, mgr *Manager, sessionID string) SessionMeta {
	t.Helper()
	meta, found, err := mgr.loadSessionMeta(sessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta(%s): err=%v found=%v", sessionID, err, found)
	}
	return meta
}

func bindRemoteSession(t *testing.T, mgr *Manager, sessionID, remoteID string) {
	t.Helper()
	meta := sessionMetaFor(t, mgr, sessionID)
	meta.AgentSessionID = remoteID
	if err := mgr.saveSessionMeta(meta); err != nil {
		t.Fatalf("saveSessionMeta(%s): %v", sessionID, err)
	}
}

// A request that names only the workspace id must still reach the canonical
// root recorded in the registry: this is the session-side half of the grant
// preflight that used to receive an empty path.
func TestWorkspaceIDAloneResolvesTheRegisteredRoot(t *testing.T) {
	router := &mockRouter{}
	mgr, _ := newWorkspaceSessionManager(t, router, workspace.Meta{ID: "ws-a", RootPath: "/srv/work/a"})

	if err := routeWorkspaceProbe(t, mgr, "ch-id-only", "ws-a", ""); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}
	meta := sessionMetaFor(t, mgr, activeSessionIDFor(t, mgr, "ch-id-only"))
	if meta.WorkspaceID != "ws-a" || meta.WorkspacePath != "/srv/work/a" {
		t.Fatalf("workspace id alone must bind the registered root, got id=%q path=%q", meta.WorkspaceID, meta.WorkspacePath)
	}
}

func TestBothWorkspaceEntrypointsBindTheSameCanonicalWorkspace(t *testing.T) {
	router := &mockRouter{}
	mgr, _ := newWorkspaceSessionManager(t, router,
		workspace.Meta{ID: "ws-a", RootPath: "/srv/work/a"},
		workspace.Meta{ID: "ws-b", RootPath: "/srv/work/b"},
	)

	if err := routeWorkspaceProbe(t, mgr, "ch-id", "ws-a", ""); err != nil {
		t.Fatalf("RouteConversation(id only): %v", err)
	}
	if err := routeWorkspaceProbe(t, mgr, "ch-id-and-path", "ws-a", "/srv/work/a"); err != nil {
		t.Fatalf("RouteConversation(id and path): %v", err)
	}
	idOnly := sessionMetaFor(t, mgr, activeSessionIDFor(t, mgr, "ch-id"))
	both := sessionMetaFor(t, mgr, activeSessionIDFor(t, mgr, "ch-id-and-path"))
	if idOnly.WorkspaceID != both.WorkspaceID || idOnly.WorkspacePath != both.WorkspacePath {
		t.Fatalf("the two entrypoints disagree: id-only=%q/%q both=%q/%q",
			idOnly.WorkspaceID, idOnly.WorkspacePath, both.WorkspaceID, both.WorkspacePath)
	}
	for _, channelID := range []string{"ch-id", "ch-id-and-path"} {
		plan, err := mgr.PlanSessionAffinity(channelID, "planner", "ws-a", "/srv/work/a")
		if err != nil {
			t.Fatalf("PlanSessionAffinity(%s): %v", channelID, err)
		}
		if plan.WorkspaceID != "ws-a" || plan.WorkspacePath != "/srv/work/a" {
			t.Fatalf("plan for %s must carry the canonical workspace, got %+v", channelID, plan)
		}
	}
}

// The regression that started issue 7: a different workspace must never receive
// the remote session another workstream created, even on a brand new channel.
func TestTwoWorkspacesDoNotShareTheRemoteSession(t *testing.T) {
	router := &mockRouter{}
	mgr, storage := newWorkspaceSessionManager(t, router,
		workspace.Meta{ID: "ws-a", RootPath: "/srv/work/a"},
		workspace.Meta{ID: "ws-b", RootPath: "/srv/work/b"},
	)

	if err := routeWorkspaceProbe(t, mgr, "ch-a", "ws-a", ""); err != nil {
		t.Fatalf("first workstream: %v", err)
	}
	sessionA := activeSessionIDFor(t, mgr, "ch-a")
	bindRemoteSession(t, mgr, sessionA, "ses-workstream-a")

	if err := routeWorkspaceProbe(t, mgr, "ch-b", "ws-b", ""); err != nil {
		t.Fatalf("second workstream: %v", err)
	}
	sessionB := activeSessionIDFor(t, mgr, "ch-b")
	if sessionB == sessionA {
		t.Fatalf("two workspaces shared the logical session %s", sessionA)
	}
	if router.lastRemote == "ses-workstream-a" {
		t.Fatalf("workspace ws-b reused the remote session of ws-a: %s", router.lastRemote)
	}
	metaB := sessionMetaFor(t, mgr, sessionB)
	if metaB.WorkspaceID != "ws-b" || metaB.WorkspacePath != "/srv/work/b" {
		t.Fatalf("second workstream bound to the wrong workspace: %+v", metaB)
	}
	indexA, err := workspace.LoadSessionIndex(storage, "ws-b")
	if err != nil {
		t.Fatalf("LoadSessionIndex(ws-b): %v", err)
	}
	for _, sessionID := range indexA {
		if sessionID == sessionA {
			t.Fatalf("ws-b index contains ws-a's session %s", sessionA)
		}
	}
}

// The decision recorded before the prompt must name the workspace and the remote
// session the routing decision reused, so adopting another workstream's session
// is visible while the turn is still running.
func TestWorkspaceDecisionNamesTheReusedRemoteSessionBeforeThePrompt(t *testing.T) {
	router := &mockRouter{}
	mgr, storage := newWorkspaceSessionManager(t, router, workspace.Meta{ID: "ws-a", RootPath: "/srv/work/a"})

	if err := routeWorkspaceProbe(t, mgr, "ch-a", "ws-a", ""); err != nil {
		t.Fatalf("first turn: %v", err)
	}
	sessionID := activeSessionIDFor(t, mgr, "ch-a")
	bindRemoteSession(t, mgr, sessionID, "ses-workstream-a")
	if err := routeWorkspaceProbe(t, mgr, "ch-a", "ws-a", ""); err != nil {
		t.Fatalf("second turn: %v", err)
	}
	if router.lastRemote != "ses-workstream-a" {
		t.Fatalf("second turn did not reuse the remote session: %q", router.lastRemote)
	}

	events, err := workspace.LoadTimeline(storage, "ws-a", 20)
	if err != nil {
		t.Fatalf("LoadTimeline: %v", err)
	}
	for _, event := range events {
		if event.Type != "decision.recorded" {
			continue
		}
		if event.Metadata["workspace_id"] != "ws-a" || event.Metadata["workspace_path"] != "/srv/work/a" ||
			event.Metadata["selected_remote_session_id"] != "ses-workstream-a" || event.Metadata["reuses_remote_session"] != true {
			t.Fatalf("decision recorded before the prompt must name the workspace and the reused remote session: %+v", event.Metadata)
		}
		return
	}
	t.Fatalf("no decision.recorded event for ws-a: %+v", events)
}

// A session whose recorded workspace path belongs to another workspace must not
// be resumed by the workspace index, even when the ids were written equal by the
// ambiguous binding the contract now refuses.
func TestSessionOfAnotherWorkspacePathIsNotReused(t *testing.T) {
	router := &mockRouter{}
	mgr, storage := newWorkspaceSessionManager(t, router,
		workspace.Meta{ID: "ws-a", RootPath: "/srv/work/a"},
		workspace.Meta{ID: "ws-b", RootPath: "/srv/work/b"},
	)
	legacy := SessionMeta{
		ID:             "legacy-ambiguous",
		AgentID:        "planner",
		AgentSessionID: "ses-legacy",
		WorkspaceID:    "ws-b",
		WorkspacePath:  "/srv/work/a",
		Status:         "active",
	}
	if err := mgr.saveSessionMeta(legacy); err != nil {
		t.Fatalf("saveSessionMeta: %v", err)
	}
	if err := workspace.UpdateSessionIndex(storage, "ws-b", legacy.ID); err != nil {
		t.Fatalf("UpdateSessionIndex: %v", err)
	}

	plan, err := mgr.PlanSessionAffinity("ch-probe", "planner", "ws-b", "/srv/work/b")
	if err != nil {
		t.Fatalf("PlanSessionAffinity: %v", err)
	}
	if plan.LogicalSessionID == legacy.ID || plan.RemoteSessionID == "ses-legacy" {
		t.Fatalf("plan would reuse a session of another workspace path: %+v", plan)
	}
	if !plan.NewSession || plan.WorkspacePath != "/srv/work/b" {
		t.Fatalf("plan must create an isolated session for the requested workspace, got %+v", plan)
	}
	if err := routeWorkspaceProbe(t, mgr, "ch-probe", "ws-b", "/srv/work/b"); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}
	if router.lastRemote == "ses-legacy" {
		t.Fatal("routing handed the other workspace's remote session to the provider")
	}
	if got := activeSessionIDFor(t, mgr, "ch-probe"); got == legacy.ID {
		t.Fatalf("routing resumed the legacy session %s", legacy.ID)
	}
}

// The active-session match is the other half of the same guard, and it is a
// different branch from the workspace index: a channel whose CURRENT session
// records the right workspace id but another workspace's directory must not keep
// that session either. Without the path comparison the id alone would match and
// the foreign session would be reused on every turn of that channel.
func TestActiveSessionOfAnotherWorkspacePathIsNotReused(t *testing.T) {
	router := &mockRouter{}
	mgr, storage := newWorkspaceSessionManager(t, router,
		workspace.Meta{ID: "ws-a", RootPath: "/srv/work/a"},
		workspace.Meta{ID: "ws-b", RootPath: "/srv/work/b"},
	)
	legacy := SessionMeta{
		ID:             "legacy-active-foreign-path",
		AgentID:        defaultAgentID,
		AgentSessionID: "ses-legacy-active",
		WorkspaceID:    "ws-b",
		WorkspacePath:  "/srv/work/a",
		Status:         "active",
	}
	if err := mgr.saveSessionMeta(legacy); err != nil {
		t.Fatalf("saveSessionMeta: %v", err)
	}
	// The record is the channel's ACTIVE session and is deliberately absent from
	// the workspace index: only the active branch can hand it out.
	if err := mgr.updateChannelState("ch-active", legacy.ID); err != nil {
		t.Fatalf("updateChannelState: %v", err)
	}
	indexed, err := workspace.LoadSessionIndex(storage, "ws-b")
	if err != nil {
		t.Fatalf("LoadSessionIndex: %v", err)
	}
	for _, sessionID := range indexed {
		if sessionID == legacy.ID {
			t.Fatalf("fixture leaked into the workspace index %s, the test would not exercise the active branch", legacy.ID)
		}
	}

	plan, err := mgr.PlanSessionAffinity("ch-active", "", "ws-b", "/srv/work/b")
	if err != nil {
		t.Fatalf("PlanSessionAffinity: %v", err)
	}
	if plan.LogicalSessionID == legacy.ID || plan.RemoteSessionID == "ses-legacy-active" {
		t.Fatalf("plan would reuse the channel's active session under another workspace path: %+v", plan)
	}
	if !plan.NewSession || plan.Kind != workspaceRouteCreate {
		t.Fatalf("plan must create a session for a foreign-path active record, got %+v", plan)
	}

	if err := routeWorkspaceProbe(t, mgr, "ch-active", "ws-b", "/srv/work/b"); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}
	if router.lastRemote == "ses-legacy-active" {
		t.Fatal("routing handed the active session's foreign-path remote session to the provider")
	}
	if got := activeSessionIDFor(t, mgr, "ch-active"); got == legacy.ID {
		t.Fatalf("routing reused the active session %s whose recorded path belongs to ws-a", legacy.ID)
	}
	if meta := sessionMetaFor(t, mgr, activeSessionIDFor(t, mgr, "ch-active")); meta.WorkspaceID != "ws-b" || meta.WorkspacePath != "/srv/work/b" {
		t.Fatalf("the new session was bound to the wrong workspace: %+v", meta)
	}
}

func TestWorkspaceIdentityMismatchIsRefusedBeforeRouting(t *testing.T) {
	router := &mockRouter{}
	mgr, _ := newWorkspaceSessionManager(t, router,
		workspace.Meta{ID: "ws-a", RootPath: "/srv/work/a"},
		workspace.Meta{ID: "ws-b", RootPath: "/srv/work/b"},
	)

	err := routeWorkspaceProbe(t, mgr, "ch-mismatch", "ws-b", "/srv/work/a")
	var mismatch *workspace.MismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("mismatched workspace hints must be a typed refusal, got %v", err)
	}
	if router.lastMsg != "" {
		t.Fatalf("a refused mismatch must not reach the provider, got %q", router.lastMsg)
	}
	if _, planErr := mgr.PlanSessionAffinity("ch-mismatch", "planner", "ws-b", "/srv/work/a"); !errors.As(planErr, &mismatch) {
		t.Fatalf("the published plan must refuse the same mismatch, got %v", planErr)
	}
}

func TestPlanSessionAffinityMatchesTheSessionThatRoutes(t *testing.T) {
	router := &mockRouter{}
	mgr, _ := newWorkspaceSessionManager(t, router,
		workspace.Meta{ID: "ws-a", RootPath: "/srv/work/a"},
		workspace.Meta{ID: "ws-b", RootPath: "/srv/work/b"},
	)
	if err := routeWorkspaceProbe(t, mgr, "ch-origin", "ws-a", ""); err != nil {
		t.Fatalf("origin workstream: %v", err)
	}
	origin := activeSessionIDFor(t, mgr, "ch-origin")
	bindRemoteSession(t, mgr, origin, "ses-origin")

	// A second channel asking for the same workspace must resume that session,
	// and the plan published before the prompt must say exactly that.
	plan, err := mgr.PlanSessionAffinity("ch-follow-up", mgr.defaultAgent, "ws-a", "/srv/work/a")
	if err != nil {
		t.Fatalf("PlanSessionAffinity: %v", err)
	}
	if plan.Kind != workspaceRouteResumeIndexed || plan.LogicalSessionID != origin || plan.RemoteSessionID != "ses-origin" || !plan.ReusesRemoteSession {
		t.Fatalf("plan does not describe the resume that routing performs: %+v", plan)
	}
	if err := routeWorkspaceProbe(t, mgr, "ch-follow-up", "ws-a", "/srv/work/a"); err != nil {
		t.Fatalf("follow-up route: %v", err)
	}
	if router.lastSession != plan.LogicalSessionID || router.lastRemote != plan.RemoteSessionID {
		t.Fatalf("plan=%s/%s but routing used=%s/%s", plan.LogicalSessionID, plan.RemoteSessionID, router.lastSession, router.lastRemote)
	}
	if got := activeSessionIDFor(t, mgr, "ch-follow-up"); got != plan.LogicalSessionID {
		t.Fatalf("channel active session %s differs from the published plan %s", got, plan.LogicalSessionID)
	}
}

func TestPlanSessionAffinityPredictsAnIsolatedSession(t *testing.T) {
	router := &mockRouter{}
	mgr, _ := newWorkspaceSessionManager(t, router, workspace.Meta{ID: "ws-a", RootPath: "/srv/work/a"})
	plan, err := mgr.PlanSessionAffinity("ch-new", "planner", "ws-a", "/srv/work/a")
	if err != nil {
		t.Fatalf("PlanSessionAffinity: %v", err)
	}
	if !plan.NewSession || plan.ReusesRemoteSession || plan.RemoteSessionID != "" {
		t.Fatalf("a first probe must be published as a new isolated session, got %+v", plan)
	}
	if plan.WorkspaceID != "ws-a" || plan.WorkspacePath != "/srv/work/a" {
		t.Fatalf("plan lost the resolved workspace: %+v", plan)
	}
}
