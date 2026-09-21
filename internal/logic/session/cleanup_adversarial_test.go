package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// ----------------------------------------------------------------------------
// Adversarial recursive-cleanup suite
// ----------------------------------------------------------------------------

func newCleanupTestManager(t *testing.T) (*Manager, *mockStorage) {
	t.Helper()
	storage := &mockStorage{data: map[string][]byte{}}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("seed configured flag: %v", err)
	}
	manager := NewManager(storage, &mockRouter{}, newTestWizard(storage), nil)
	return manager, storage
}

func saveMeta(t *testing.T, storage *mockStorage, meta SessionMeta) {
	t.Helper()
	payload, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := storage.Set(getSessionKey(meta.ID), payload); err != nil {
		t.Fatalf("store meta: %v", err)
	}
}

func saveChannelState(t *testing.T, storage *mockStorage, channelID, activeSessionID string) {
	t.Helper()
	payload, err := json.Marshal(ChannelState{ActiveSessionID: activeSessionID})
	if err != nil {
		t.Fatalf("marshal channel state: %v", err)
	}
	if err := storage.Set("session.channel."+channelID, payload); err != nil {
		t.Fatalf("store channel state: %v", err)
	}
}

func metaExists(t *testing.T, storage *mockStorage, sessionID string) bool {
	t.Helper()
	raw, err := storage.Get(getSessionKey(sessionID))
	return err == nil && len(raw) > 0
}

// TestForkCleanupSurvivesACyclicGraph is the regression guard for the missing
// cycle guard: a fork graph whose parent and child reference each other's remote
// session used to recurse until the stack died.
func TestForkCleanupSurvivesACyclicGraph(t *testing.T) {
	manager, storage := newCleanupTestManager(t)

	parent := SessionMeta{
		ID: "parent", AgentID: "codex", AgentSessionID: "R1",
		ParentRemoteID: "R2", // points back at the child's remote session
		WorkspacePath:  "/tmp/repo",
	}
	child := SessionMeta{
		ID: "child", AgentID: "codex", AgentSessionID: "R2",
		ParentSessionID: "parent", ParentRemoteID: "R1",
		WorkspacePath: "/tmp/repo",
	}
	saveMeta(t, storage, parent)
	saveMeta(t, storage, child)

	done := make(chan middleware.SessionCleanupResult, 1)
	go func() {
		done <- manager.cleanupSessionMirrorAndRemote(context.Background(), sessionCleanupExecution{
			ChannelID:        "telegram_1",
			Meta:             parent,
			ForceForgetLocal: true,
		})
	}()

	select {
	case result := <-done:
		if len(result.ForkChildren) == 0 {
			t.Fatal("the cyclic child must still be reported")
		}
		// The visited guard must stop the cycle at the second session, not rely
		// on the depth backstop: a deeper tree means the same sessions are being
		// cleaned over and over.
		if depth := forkResultDepth(result); depth > 3 {
			t.Fatalf("cyclic cleanup recursed to depth %d; sessions are revisited", depth)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("recursive fork cleanup did not terminate on a cyclic graph")
	}
}

// forkResultDepth returns how deeply fork cleanups nest in a result.
func forkResultDepth(result middleware.SessionCleanupResult) int {
	depth := 0
	for _, child := range result.ForkChildren {
		if candidate := 1 + forkResultDepth(child); candidate > depth {
			depth = candidate
		}
	}
	return depth
}

// TestForkCleanupVisitsEachSessionOnce proves the guard is keyed on identity and
// not merely on recursion depth.
func TestForkCleanupVisitsEachSessionOnce(t *testing.T) {
	visited := newCleanupVisited()
	first := SessionMeta{ID: "a", AgentSessionID: "R1"}
	if !visited.enter(first) {
		t.Fatal("the first visit of a session must be allowed")
	}
	if visited.enter(first) {
		t.Fatal("the same session must not be entered twice")
	}
	// A different logical id for the same remote session is still a repeat.
	if visited.enter(SessionMeta{ID: "b", AgentSessionID: "R1"}) {
		t.Fatal("a second logical id for the same remote session must be refused")
	}
	if !visited.enter(SessionMeta{ID: "c", AgentSessionID: "R3"}) {
		t.Fatal("a distinct session must be allowed")
	}
	// Logical identity alone is enough.
	if visited.enter(SessionMeta{ID: "a", AgentSessionID: ""}) {
		t.Fatal("a repeat logical id must be refused")
	}
}

// TestForkCleanupDoesNotDeleteAnActiveChild keeps a channel's live session from
// being destroyed as a side effect of cleaning its parent.
func TestForkCleanupDoesNotDeleteAnActiveChild(t *testing.T) {
	manager, storage := newCleanupTestManager(t)

	parent := SessionMeta{ID: "parent", AgentID: "codex", AgentSessionID: "R1"}
	child := SessionMeta{ID: "child", AgentID: "codex", AgentSessionID: "R2", ParentSessionID: "parent"}
	saveMeta(t, storage, parent)
	saveMeta(t, storage, child)
	saveChannelState(t, storage, "telegram_1", "child")

	result := manager.cleanupSessionMirrorAndRemote(context.Background(), sessionCleanupExecution{
		ChannelID:        "telegram_1",
		Meta:             parent,
		ForceForgetLocal: true,
	})

	if !metaExists(t, storage, "child") {
		t.Fatal("a fork child that is active in a channel must not be deleted")
	}
	reported := false
	for _, childResult := range result.ForkChildren {
		if childResult.LogicalSessionID == "child" {
			reported = true
			if childResult.Clean {
				t.Fatal("an active child must not be reported as cleaned")
			}
		}
	}
	if !reported {
		t.Fatal("the retained child must be reported in the result")
	}
}

// TestSessionActiveInAnyChannelIsAccurate keeps the guard both destructive
// protections rely on honest.
func TestSessionActiveInAnyChannelIsAccurate(t *testing.T) {
	manager, storage := newCleanupTestManager(t)
	saveChannelState(t, storage, "telegram_1", "sess-active")

	if !manager.sessionActiveInAnyChannel("sess-active") {
		t.Fatal("an active session must be recognised")
	}
	if manager.sessionActiveInAnyChannel("sess-other") {
		t.Fatal("an idle session must not be reported as active")
	}
	if manager.sessionActiveInAnyChannel("") {
		t.Fatal("an empty id is never active")
	}
}

// TestCleanupAlreadyHandledResultIsHonest pins what a skipped session reports:
// handled sessions are clean, while an abandoned graph is a failure.
func TestCleanupAlreadyHandledResultIsHonest(t *testing.T) {
	req := sessionCleanupExecution{Meta: SessionMeta{ID: "s", AgentSessionID: "R"}}
	skipped := cleanupAlreadyHandledResult(req, middleware.SessionCleanupPolicyDeleteRemote, false)
	if !skipped.Clean {
		t.Fatal("a session already handled in this cleanup must report clean")
	}
	if skipped.Warnings == nil {
		t.Fatal("a skipped cycle must be reported as a warning")
	}

	abandoned := cleanupAlreadyHandledResult(req, middleware.SessionCleanupPolicyDeleteRemote, true)
	if abandoned.Clean {
		t.Fatal("an abandoned graph must not report a clean result")
	}
	if abandoned.Error == "" {
		t.Fatal("an abandoned graph must explain itself")
	}
}

// TestActiveAgentClientRefsKeepDistinctRemoteSessions is the regression guard
// for live-client reaping: two sessions for the same agent and workspace but
// different remote sessions must both be reported, or reconcile closes the
// client of whichever one was dropped.
func TestActiveAgentClientRefsKeepDistinctRemoteSessions(t *testing.T) {
	manager, storage := newCleanupTestManager(t)

	saveMeta(t, storage, SessionMeta{
		ID: "s1", AgentID: "codex", AgentSessionID: "remote-1",
		WorkspacePath: "/tmp/repo", ProtocolKind: "acp",
	})
	saveMeta(t, storage, SessionMeta{
		ID: "s2", AgentID: "codex", AgentSessionID: "remote-2",
		WorkspacePath: "/tmp/repo", ProtocolKind: "acp",
	})

	refs, err := manager.activeAgentClientRefs()
	if err != nil {
		t.Fatalf("activeAgentClientRefs: %v", err)
	}
	remotes := map[string]bool{}
	for _, ref := range refs {
		remotes[ref.RemoteSessionID] = true
	}
	if !remotes["remote-1"] || !remotes["remote-2"] {
		t.Fatalf("both live remote sessions must be reported, got %+v", refs)
	}
}

// TestActiveAgentClientRefsStillDeduplicateExactRepeats keeps the dedupe useful:
// identical ownership must not be reported twice.
func TestActiveAgentClientRefsStillDeduplicateExactRepeats(t *testing.T) {
	manager, storage := newCleanupTestManager(t)
	saveMeta(t, storage, SessionMeta{
		ID: "s1", AgentID: "codex", AgentSessionID: "remote-1", WorkspacePath: "/tmp/repo",
	})
	refs, err := manager.activeAgentClientRefs()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected one ref, got %+v", refs)
	}
	if refs[0].LogicalSessionID != "s1" || refs[0].RemoteSessionID != "remote-1" {
		t.Fatalf("unexpected ref: %+v", refs[0])
	}
}

// TestActiveAgentClientRefsSkipIncompleteSessions keeps sessions without a
// remote session out of the reconcile set.
func TestActiveAgentClientRefsSkipIncompleteSessions(t *testing.T) {
	manager, storage := newCleanupTestManager(t)
	saveMeta(t, storage, SessionMeta{ID: "no-agent", AgentSessionID: "remote-1"})
	saveMeta(t, storage, SessionMeta{ID: "no-remote", AgentID: "codex"})
	refs, err := manager.activeAgentClientRefs()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("incomplete sessions must be skipped, got %+v", refs)
	}
}

// TestConcurrentFirstMessagesCreateOneSession is the regression guard for the
// channel-state lost update: two simultaneous first messages used to create two
// sessions and leave one unreferenced.
func TestConcurrentFirstMessagesCreateOneSession(t *testing.T) {
	manager, storage := newCleanupTestManager(t)

	const callers = 8
	ids := make(chan string, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			sessionID, err := manager.GetOrCreateSession("telegram_1", "codex")
			if err != nil {
				t.Errorf("GetOrCreateSession: %v", err)
				return
			}
			ids <- sessionID
		}()
	}
	close(start)
	wg.Wait()
	close(ids)

	distinct := map[string]int{}
	for id := range ids {
		distinct[id]++
	}
	if len(distinct) != 1 {
		t.Fatalf("concurrent first messages created %d sessions: %v", len(distinct), distinct)
	}

	state, err := manager.getChannelState("telegram_1")
	if err != nil {
		t.Fatalf("channel state: %v", err)
	}
	if len(state.History) != 1 {
		t.Fatalf("history must reference the single created session, got %v", state.History)
	}
	// Any session created but not referenced by the channel would be leaked.
	entries, err := storage.List("session.meta.")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one session record, got %d (%v)", len(entries), entries)
	}
}

// TestUpdateChannelStateKeepsHistoryConsistent proves the read-modify-write
// cannot lose a concurrent workspace update.
func TestUpdateChannelStateKeepsHistoryConsistent(t *testing.T) {
	manager, _ := newCleanupTestManager(t)

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			if err := manager.updateChannelWorkspaceState("telegram_1", "ws-1"); err != nil {
				t.Errorf("workspace update: %v", err)
			}
		}(i)
	}
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			if err := manager.updateChannelState("telegram_1", "sess-"+string(rune('a'+index))); err != nil {
				t.Errorf("state update: %v", err)
			}
		}(i)
	}
	wg.Wait()

	state, err := manager.getChannelState("telegram_1")
	if err != nil {
		t.Fatalf("channel state: %v", err)
	}
	if state.PreferredWorkspaceID != "ws-1" {
		t.Fatalf("the workspace mapping was lost by a concurrent write: %+v", state)
	}
	if state.ActiveSessionID == "" {
		t.Fatalf("the active session was lost: %+v", state)
	}
}

// TestApplyPendingHandoffDoesNotDropANewerRemoteSession guards the stale-snapshot
// overwrite: the snapshot taken at turn start must not erase the remote session
// the queue persisted while the turn ran.
func TestApplyPendingHandoffDoesNotDropANewerRemoteSession(t *testing.T) {
	manager, storage := newCleanupTestManager(t)

	// The snapshot as the turn saw it: no remote session yet.
	snapshot := SessionMeta{
		ID: "sess-1", AgentID: "codex", Status: "active",
		PendingHandoff: &middleware.HandoffPacket{ToAgentID: "claude"},
	}
	// Meanwhile the queue recorded the remote session.
	saveMeta(t, storage, SessionMeta{
		ID: "sess-1", AgentID: "codex", Status: "active",
		AgentSessionID: "remote-live",
	})

	manager.applyPendingHandoff(&snapshot, "telegram_1", slog.Default(), nil)

	stored, found, err := manager.loadSessionMeta("sess-1")
	if err != nil || !found {
		t.Fatalf("load meta: %v %v", found, err)
	}
	if stored.AgentSessionID != "remote-live" {
		t.Fatalf("the newer remote session was dropped: %q", stored.AgentSessionID)
	}
	if stored.PendingHandoff != nil {
		t.Fatal("the pending handoff must be cleared")
	}
	if stored.LastHandoff == nil {
		t.Fatal("the applied handoff must be recorded")
	}
}
