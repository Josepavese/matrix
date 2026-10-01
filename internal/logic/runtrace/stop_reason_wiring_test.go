package runtrace

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

// recordingNotifier stands in for the run's notifier: it is the capability a turn
// uses to report what a provider said ended it, without the run layer knowing
// which protocol said it.
type recordingNotifier struct {
	store *Store
	runID string
}

// OnTurnStopReason records the reason as a fact of the turn it belongs to, which
// is what lets the terminal transition read it without being told again.
func (n *recordingNotifier) OnTurnStopReason(stopReason string) {
	_, _ = n.store.AppendEvent(Event{
		RunID: n.runID, Kind: KindTurnStopReason,
		Metadata: map[string]interface{}{StopReasonMetadataKey: stopReason},
	})
}

// TestReportedStopReasonReachesTheRunRecord proves the peer's own word survives
// the whole path: a turn reports it, the run's notifier records it as a fact of
// that turn, and the terminal transition reads it back instead of falling back
// to a reason Matrix chose.
func TestReportedStopReasonReachesTheRunRecord(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)

	var notifier TurnStopReasonReporter = &recordingNotifier{store: store, runID: run.ID}
	notifier.OnTurnStopReason("max_tokens")

	// A turn that produced something terminalizes without being told the reason:
	// the run reads it from the turn's own record.
	completed, err := store.Complete(run.ID, "a partial answer", "")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.StopReason != "max_tokens" {
		t.Fatalf("the recorded stop reason did not reach the run: %q", completed.StopReason)
	}
	if completed.Status != StatusCompleted {
		t.Fatalf("status = %q", completed.Status)
	}
}

// TestUnreportedStopReasonStaysUnreported proves the other half: when nothing
// reported a reason, the run says so. This is the assertion that fails if Matrix
// ever again fills the gap with "end_turn".
func TestUnreportedStopReasonStaysUnreported(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)
	completed, err := store.Complete(run.ID, "an answer", "")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.StopReason != StopReasonReportedNone {
		t.Fatalf("stop reason = %q, want %q", completed.StopReason, StopReasonReportedNone)
	}
	if IsReportedStopReason(completed.StopReason) {
		t.Fatal("an absent reason was reported as one the peer stated")
	}
}

// TestCallerSuppliedReasonWinsOverTheRecordedOne keeps the precedence explicit:
// a caller that knows the reason for this transition states it, and the turn's
// own record is the fallback rather than an override.
func TestCallerSuppliedReasonWinsOverTheRecordedOne(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)
	if _, err := store.AppendEvent(Event{
		RunID: run.ID, Kind: KindTurnStopReason,
		Metadata: map[string]interface{}{StopReasonMetadataKey: "max_tokens"},
	}); err != nil {
		t.Fatalf("append turn reason: %v", err)
	}
	completed, err := store.Complete(run.ID, "answer", "end_turn")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.StopReason != "end_turn" {
		t.Fatalf("the caller's reason was overridden: %q", completed.StopReason)
	}
}
