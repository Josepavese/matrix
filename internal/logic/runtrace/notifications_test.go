package runtrace

import (
	"sync"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

func TestNotificationsPersistMinimalEventsAcrossRestart(t *testing.T) {
	storage := memstore.New()
	store := NewStore(storage)
	for _, runID := range []string{"run-one", "run-two", "run-three"} {
		run, _, err := store.Start(Run{ID: runID, AgentID: "agent", ChannelID: "supervisor"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Complete(run.ID, "private output", "end_turn"); err != nil {
			t.Fatal(err)
		}
	}
	restarted := NewStore(storage)
	items, cursor, err := restarted.LoadNotificationsAfter(0, 100, nil)
	if err != nil || len(items) != 3 || cursor != 3 {
		t.Fatalf("notifications=%+v cursor=%d err=%v", items, cursor, err)
	}
	for _, item := range items {
		if item.Kind != "run.completed" || item.RunID == "" || item.AgentID != "" {
			t.Fatalf("unexpected content in wakeup: %+v", item)
		}
	}
	filtered, cursor, err := restarted.LoadNotificationsAfter(0, 100, map[string]struct{}{"run-two": {}})
	if err != nil || len(filtered) != 1 || filtered[0].RunID != "run-two" || cursor != 3 {
		t.Fatalf("filtered=%+v cursor=%d err=%v", filtered, cursor, err)
	}
	latest, _, err := restarted.LoadNotificationsAfter(3, 100, nil)
	if err != nil || len(latest) != 0 {
		t.Fatalf("replayed past cursor: %+v %v", latest, err)
	}
}

func TestConcurrentTerminalNotificationsAndStartupRepair(t *testing.T) {
	storage := memstore.New()
	store := NewStore(storage)
	ids := []string{"run-a", "run-b", "run-c"}
	for _, id := range ids {
		if _, _, err := store.Start(Run{ID: id, AgentID: "agent"}); err != nil {
			t.Fatal(err)
		}
	}
	var group sync.WaitGroup
	for _, id := range ids {
		group.Add(1)
		go func(id string) {
			defer group.Done()
			if _, err := store.Complete(id, "private output", "end_turn"); err != nil {
				t.Errorf("complete %s: %v", id, err)
			}
		}(id)
	}
	group.Wait()
	items, _, err := store.LoadNotificationsAfter(0, 100, nil)
	if err != nil || len(items) != len(ids) {
		t.Fatalf("concurrent notifications=%+v err=%v", items, err)
	}
	// Simulate a terminal run committed immediately before the wakeup write.
	if _, _, err := store.Start(Run{ID: "run-missed", AgentID: "agent"}); err != nil {
		t.Fatal(err)
	}
	run, found, err := store.LoadRun("run-missed")
	if err != nil || !found {
		t.Fatal(err)
	}
	run.Status = StatusCompleted
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	restarted := NewStore(storage)
	if repaired, err := restarted.ReconcileTerminalNotifications(); err != nil || repaired != 1 {
		t.Fatalf("repaired=%d err=%v", repaired, err)
	}
	if repaired, err := restarted.ReconcileTerminalNotifications(); err != nil || repaired != 0 {
		t.Fatalf("second repair=%d err=%v", repaired, err)
	}
	items, _, err = restarted.LoadNotificationsAfter(0, 100, nil)
	if err != nil || len(items) != 4 {
		t.Fatalf("repaired notifications=%+v err=%v", items, err)
	}
}

func TestNotificationSequenceRecoversPastStaleCounter(t *testing.T) {
	storage := memstore.New()
	first := NewStore(storage)
	if _, err := first.AppendNotification(Notification{Kind: "run.completed", RunID: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := storage.Set(notificationSequenceKey, []byte("0")); err != nil {
		t.Fatal(err)
	}
	second := NewStore(storage)
	item, err := second.AppendNotification(Notification{Kind: "run.completed", RunID: "second"})
	if err != nil || item.Sequence != 2 {
		t.Fatalf("sequence=%d err=%v", item.Sequence, err)
	}
	items, _, err := second.LoadNotificationsAfter(0, 100, nil)
	if err != nil || len(items) != 2 || items[0].RunID != "first" || items[1].RunID != "second" {
		t.Fatalf("notifications=%+v err=%v", items, err)
	}
}

func TestFindRunningRunForSessionRequiresUnambiguousAgentAndRemoteSession(t *testing.T) {
	store := NewStore(memstore.New())
	start := func(id, agent, remote string) {
		t.Helper()
		if _, _, err := store.Start(Run{ID: id, AgentID: agent, RemoteSessionID: remote, ChannelID: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	start("first", "dsh", "remote-a")
	start("other-agent", "codex", "remote-a")
	start("other-session", "dsh", "remote-b")
	if got := store.FindRunningRunForSession("dsh", "remote-a"); got != "first" {
		t.Fatalf("unique running match = %q, want first", got)
	}
	if got := store.FindRunningRunForSession("", "remote-a"); got != "" {
		t.Fatalf("empty agent matched %q", got)
	}
	if got := store.FindRunningRunForSession("dsh", ""); got != "" {
		t.Fatalf("empty session matched %q", got)
	}
	start("ambiguous", "dsh", "remote-a")
	if got := store.FindRunningRunForSession("dsh", "remote-a"); got != "" {
		t.Fatalf("ambiguous session attributed to %q", got)
	}
	if _, err := store.Complete("ambiguous", "done", "end_turn"); err != nil {
		t.Fatal(err)
	}
	if got := store.FindRunningRunForSession("dsh", "remote-a"); got != "first" {
		t.Fatalf("terminal run obscured unique running match: %q", got)
	}
	if _, err := store.Complete("first", "done", "end_turn"); err != nil {
		t.Fatal(err)
	}
	if got := store.FindRunningRunForSession("dsh", "remote-a"); got != "" {
		t.Fatalf("terminal session matched %q", got)
	}
}
