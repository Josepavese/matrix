package workspace

import (
	"testing"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

type mockStorage struct {
	data map[string][]byte
}

func (m *mockStorage) Get(key string) ([]byte, error) {
	return m.data[key], nil
}

func (m *mockStorage) Set(key string, val []byte) error {
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	m.data[key] = val
	return nil
}

func (m *mockStorage) Delete(key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockStorage) List(prefix string) ([]string, error) {
	var keys []string
	for k := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

var _ middleware.Storage = (*mockStorage)(nil)

func TestSaveAndResolveByPath(t *testing.T) {
	store := &mockStorage{data: make(map[string][]byte)}
	if err := SaveMeta(store, Meta{
		ID:       "billing-api",
		Name:     "billing-api",
		RootPath: "/tmp/billing-api",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	meta, found, err := ResolveByPath(store, "/tmp/billing-api")
	if err != nil {
		t.Fatalf("ResolveByPath: %v", err)
	}
	if !found {
		t.Fatal("expected workspace to be resolved by path")
	}
	if meta.ID != "billing-api" {
		t.Fatalf("expected workspace id billing-api, got %q", meta.ID)
	}
}

func TestSessionIndexKeepsMostRecentFirst(t *testing.T) {
	store := &mockStorage{data: make(map[string][]byte)}
	if err := UpdateSessionIndex(store, "billing-api", "s1"); err != nil {
		t.Fatalf("UpdateSessionIndex(s1): %v", err)
	}
	if err := UpdateSessionIndex(store, "billing-api", "s2"); err != nil {
		t.Fatalf("UpdateSessionIndex(s2): %v", err)
	}
	if err := UpdateSessionIndex(store, "billing-api", "s1"); err != nil {
		t.Fatalf("UpdateSessionIndex(s1 again): %v", err)
	}
	got, err := LoadSessionIndex(store, "billing-api")
	if err != nil {
		t.Fatalf("LoadSessionIndex: %v", err)
	}
	if len(got) != 2 || got[0] != "s1" || got[1] != "s2" {
		t.Fatalf("unexpected session index ordering: %+v", got)
	}
}

func TestRecordEventAndLoadTimelineNewestFirst(t *testing.T) {
	store := &mockStorage{data: make(map[string][]byte)}
	first, err := RecordEvent(store, Event{
		WorkspaceID: "billing-api",
		Type:        "session.created",
		AgentID:     "codex",
		CreatedAt:   time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordEvent(first): %v", err)
	}
	second, err := RecordEvent(store, Event{
		WorkspaceID: "billing-api",
		Type:        "handoff.created",
		AgentID:     "claude",
		CreatedAt:   time.Date(2026, 4, 15, 12, 5, 0, 0, time.UTC),
		Metadata: map[string]interface{}{
			"from_agent_id": "codex",
			"to_agent_id":   "claude",
			"summary":       "Review the billing patch",
		},
	})
	if err != nil {
		t.Fatalf("RecordEvent(second): %v", err)
	}
	got, err := LoadTimeline(store, "billing-api", 10)
	if err != nil {
		t.Fatalf("LoadTimeline: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if got[0].ID != second.ID || got[1].ID != first.ID {
		t.Fatalf("unexpected timeline order: %+v", got)
	}
}

func TestRecordEventMaterializesWorkspaceState(t *testing.T) {
	store := &mockStorage{data: make(map[string][]byte)}
	if _, err := RecordEvent(store, Event{
		WorkspaceID:      "billing-api",
		Type:             "session.created",
		LogicalSessionID: "sess-1",
		RemoteSessionID:  "remote-1",
		AgentID:          "codex",
		Mode:             "implementation",
		Message:          "Created coding session",
		CreatedAt:        time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("RecordEvent(session.created): %v", err)
	}
	if _, err := RecordEvent(store, Event{
		WorkspaceID:      "billing-api",
		Type:             "handoff.created",
		LogicalSessionID: "sess-2",
		RemoteSessionID:  "remote-2",
		AgentID:          "claude",
		Mode:             "review",
		Message:          "Handed off to review specialist",
		Metadata: map[string]interface{}{
			"from_agent_id": "codex",
			"to_agent_id":   "claude",
			"summary":       "Review the billing patch",
			"remote_status": "active",
		},
		CreatedAt: time.Date(2026, 4, 15, 12, 5, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("RecordEvent(handoff.created): %v", err)
	}

	state, found, err := LoadState(store, "billing-api")
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !found {
		t.Fatal("expected workspace state to exist")
	}
	if state.ActiveLogicalSessionID != "sess-2" {
		t.Fatalf("expected active session sess-2, got %q", state.ActiveLogicalSessionID)
	}
	if state.ActiveAgentID != "claude" {
		t.Fatalf("expected active agent claude, got %q", state.ActiveAgentID)
	}
	if state.ActiveMode != "review" {
		t.Fatalf("expected active mode review, got %q", state.ActiveMode)
	}
	if state.LastEventType != "handoff.created" {
		t.Fatalf("expected last event handoff.created, got %q", state.LastEventType)
	}
	if state.RemoteStatus != "active" {
		t.Fatalf("expected remote status active, got %q", state.RemoteStatus)
	}
	if state.LastHandoff["from_agent_id"] != "codex" || state.LastHandoff["to_agent_id"] != "claude" {
		t.Fatalf("unexpected handoff state: %+v", state.LastHandoff)
	}
}

func TestRecordTurnAndSnapshotLifecycle(t *testing.T) {
	store := &mockStorage{data: make(map[string][]byte)}
	first, err := RecordTurn(store, Turn{
		WorkspaceID:      "billing-api",
		LogicalSessionID: "sess-1",
		AgentID:          "codex",
		Role:             "user",
		Content:          "Implement the billing retry fix",
	})
	if err != nil {
		t.Fatalf("RecordTurn(first): %v", err)
	}
	second, err := RecordTurn(store, Turn{
		WorkspaceID:      "billing-api",
		LogicalSessionID: "sess-1",
		AgentID:          "codex",
		Role:             "assistant",
		Content:          "I updated the retry logic.",
	})
	if err != nil {
		t.Fatalf("RecordTurn(second): %v", err)
	}
	turns, err := LoadTurns(store, "billing-api", 10)
	if err != nil {
		t.Fatalf("LoadTurns: %v", err)
	}
	if len(turns) != 2 || turns[0].ID != second.ID || turns[1].ID != first.ID {
		t.Fatalf("unexpected turns ordering: %+v", turns)
	}

	snapshot, err := SaveSnapshot(store, Snapshot{
		WorkspaceID:            "billing-api",
		Title:                  "billing retry snapshot",
		ActiveLogicalSessionID: "sess-1",
		ActiveAgentID:          "codex",
		ActiveMode:             "implementation",
		TurnIDs:                []string{second.ID, first.ID},
	})
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	snapshots, err := LoadSnapshots(store, "billing-api", 10)
	if err != nil {
		t.Fatalf("LoadSnapshots: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].ID != snapshot.ID {
		t.Fatalf("unexpected snapshots: %+v", snapshots)
	}
}
