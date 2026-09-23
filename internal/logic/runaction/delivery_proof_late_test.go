package runaction

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

// TestLateProofIsRecordedWhenTheWatcherLeavesOnCancellation pins the path that
// made TestAttachContextMarksLateWhenProviderDoesNotReturn flaky: the watcher used
// to record the terminal proof only on its ticker branch, so a run that completed
// and cancelled the watcher before the next tick left the delivery with no proof at
// all. This calls the exit-path helper directly, with the run already completed,
// which is deterministic where the race was not.
func TestLateProofIsRecordedWhenTheWatcherLeavesOnCancellation(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID:          "codex",
		Protocol:         "acp",
		ChannelID:        "noema.http",
		ExecutionMode:    runtrace.ExecutionModeAsync,
		LogicalSessionID: "logical-late",
		RemoteSessionID:  "remote-late",
	})
	if err != nil {
		t.Fatalf("Start run: %v", err)
	}
	if _, err := store.Complete(run.ID, "final", "end_turn"); err != nil {
		t.Fatalf("Complete run: %v", err)
	}

	recordings := 0
	watch := deliveryWatch{
		run:        run,
		deliveryID: "delivery-late",
		cancel:     func() {},
		recordLate: func(runtrace.Run, deliveryState, bool) { recordings++ },
	}
	service := New(store, fakeAttacher{}, nil)

	if !service.recordLateProofIfRunStopped(watch) {
		t.Fatal("a completed run must be reported as stopped, so the proof is written")
	}
	if recordings != 1 {
		t.Fatalf("the terminal proof must be recorded exactly once, got %d", recordings)
	}

	// A running run must not be marked: the attach is still in flight and owns its
	// own state.
	running, _, err := store.Start(runtrace.Run{
		AgentID:          "codex",
		Protocol:         "acp",
		ChannelID:        "noema.http",
		ExecutionMode:    runtrace.ExecutionModeAsync,
		LogicalSessionID: "logical-running",
		RemoteSessionID:  "remote-running",
	})
	if err != nil {
		t.Fatalf("Start second run: %v", err)
	}
	runningWatch := watch
	runningWatch.run = running
	if service.recordLateProofIfRunStopped(runningWatch) {
		t.Fatal("a run that is still running must not be marked late")
	}
	if recordings != 1 {
		t.Fatalf("no extra recording may happen for a running run, got %d", recordings)
	}
}
