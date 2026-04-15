package workspace

import (
	"testing"
	"time"
)

func TestPruneWorkspaceTrimsTimelineTurnsAndSnapshots(t *testing.T) {
	store := &mockStorage{data: map[string][]byte{}}
	if err := SaveMeta(store, Meta{ID: "billing-api", Name: "billing-api"}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := RecordEvent(store, Event{
			WorkspaceID: "billing-api",
			Type:        "session.created",
			Message:     "event",
			CreatedAt:   time.Now().UTC().Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("RecordEvent(%d): %v", i, err)
		}
		if _, err := RecordTurn(store, Turn{
			WorkspaceID: "billing-api",
			Role:        "user",
			Content:     "turn",
			CreatedAt:   time.Now().UTC().Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("RecordTurn(%d): %v", i, err)
		}
		if _, err := SaveSnapshot(store, Snapshot{
			WorkspaceID: "billing-api",
			Title:       "snapshot",
			CreatedAt:   time.Now().UTC().Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("SaveSnapshot(%d): %v", i, err)
		}
	}

	report, err := PruneWorkspace(store, "billing-api", RetentionPolicy{TimelineMax: 2, MemoryMax: 2, SnapshotsMax: 2})
	if err != nil {
		t.Fatalf("PruneWorkspace: %v", err)
	}
	if report.TimelineRemoved != 1 || report.MemoryRemoved != 1 || report.SnapshotsRemoved != 1 {
		t.Fatalf("unexpected prune report: %+v", report)
	}

	events, err := LoadTimeline(store, "billing-api", 10)
	if err != nil {
		t.Fatalf("LoadTimeline: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events after prune, got %d", len(events))
	}
	turns, err := LoadTurns(store, "billing-api", 10)
	if err != nil {
		t.Fatalf("LoadTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns after prune, got %d", len(turns))
	}
	snapshots, err := LoadSnapshots(store, "billing-api", 10)
	if err != nil {
		t.Fatalf("LoadSnapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots after prune, got %d", len(snapshots))
	}
}
