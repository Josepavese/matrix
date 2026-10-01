package runtrace

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

// ----------------------------------------------------------------------------
// Outcome semantics: what a completed provider turn means
// ----------------------------------------------------------------------------

// TestEmptyTurnIsNotReportedAsCompleted is the regression guard for the runs that
// reached "completed" carrying nothing at all. A peer whose turn produced no
// output, no tool call and no message must not be reported as a success, and the
// stop reason must not be manufactured either: the peer reported none, and the
// record says so.
func TestEmptyTurnIsNotReportedAsCompleted(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)

	completed, err := store.Complete(run.ID, "", "")
	if err != nil {
		t.Fatalf("complete empty turn: %v", err)
	}
	if completed.Status == StatusCompleted {
		t.Fatalf("a turn with no output, no tool call and no message was reported as %q", completed.Status)
	}
	if completed.Status != StatusFailed {
		t.Fatalf("expected the empty turn to fail, got %q", completed.Status)
	}
	if completed.StopReason == "end_turn" {
		t.Fatal("Matrix invented end_turn for a peer that reported no stop reason")
	}
	if completed.StopReason != StopReasonReportedNone {
		t.Fatalf("expected the absent stop reason to be recorded as %q, got %q", StopReasonReportedNone, completed.StopReason)
	}
	if completed.Error == "" {
		t.Fatal("expected the empty turn to carry an explanation")
	}

	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	if hasEventKind(events, "run.completed") {
		t.Fatalf("an empty turn emitted run.completed: %+v", events)
	}
	failed := findEvent(events, "run.failed")
	if failed == nil {
		t.Fatalf("expected a run.failed event for the empty turn, got %+v", events)
	}
	if failed.Metadata["failure_code"] != completionFailureCode {
		t.Fatalf("expected failure_code %q, got %v", completionFailureCode, failed.Metadata)
	}
	if hasEventKind(events, "agent.message.final") {
		t.Fatalf("an empty turn emitted a final message: %+v", events)
	}
}

// TestEmptyTurnDoesNotEmitASuccessNotification proves the wakeup a supervisor
// receives is the distinguishable one: a consumer that only watches terminal
// events must not have to read the run body to learn the turn produced nothing.
func TestEmptyTurnDoesNotEmitASuccessNotification(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)
	if _, err := store.Complete(run.ID, "", ""); err != nil {
		t.Fatalf("complete empty turn: %v", err)
	}
	notifications, _, err := store.LoadNotificationsAfter(0, 100, nil)
	if err != nil {
		t.Fatalf("load notifications: %v", err)
	}
	for _, notification := range notifications {
		if notification.RunID != run.ID {
			continue
		}
		if notification.Kind == "run.completed" {
			t.Fatalf("empty turn produced a run.completed wakeup: %+v", notifications)
		}
		if notification.Kind == "run.failed" {
			return
		}
	}
	t.Fatalf("expected a run.failed wakeup for the empty turn, got %+v", notifications)
}

// TestReportedStopReasonIsPreserved proves the other half of the rule: a peer
// that does report a stop reason keeps its own word, and a turn that answered in
// prose, or in tool calls without prose, is still a completed run.
func TestReportedStopReasonIsPreserved(t *testing.T) {
	for _, tc := range []struct {
		name       string
		output     string
		stopReason string
		events     []Event
		wantStatus string
		wantReason string
	}{
		{
			name: "reported end_turn with output", output: "the answer", stopReason: "end_turn",
			wantStatus: StatusCompleted, wantReason: "end_turn",
		},
		{
			name: "reported max_tokens with output", output: "truncated", stopReason: "max_tokens",
			wantStatus: StatusCompleted, wantReason: "max_tokens",
		},
		{
			name:   "tool call only, no final prose",
			events: []Event{{Kind: "tool.call.requested"}, {Kind: "tool.result.received"}},
			// A turn that ran tools did work even when it produced no closing
			// message, so it stays a completed run with an absent stop reason.
			wantStatus: StatusCompleted, wantReason: StopReasonReportedNone,
		},
		{
			name:   "streamed message without output field",
			events: []Event{{Kind: "agent.message.delta"}},
			// The rule reads event kinds only, never text: a message delta is
			// evidence of a turn whatever its content says.
			wantStatus: StatusCompleted, wantReason: StopReasonReportedNone,
		},
		{
			name:   "provider reported a reason Matrix does not know",
			output: "answer", stopReason: "refusal",
			wantStatus: StatusCompleted, wantReason: "refusal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := NewStore(memstore.New())
			run := startTestRun(t, store)
			for _, event := range tc.events {
				event.RunID = run.ID
				if _, err := store.AppendEvent(event); err != nil {
					t.Fatalf("append event: %v", err)
				}
			}
			completed, err := store.Complete(run.ID, tc.output, tc.stopReason)
			if err != nil {
				t.Fatalf("complete: %v", err)
			}
			if completed.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", completed.Status, tc.wantStatus)
			}
			if completed.StopReason != tc.wantReason {
				t.Fatalf("stop reason = %q, want %q", completed.StopReason, tc.wantReason)
			}
		})
	}
}

// TestTurnEvidenceIgnoresMatrixBookkeeping proves the evidence rule cannot be
// satisfied by the run's own operational events: routing, model selection and
// the prompt record exist for every run, including the ones that produced
// nothing, so counting them would restore the bug this rule closes.
func TestTurnEvidenceIgnoresMatrixBookkeeping(t *testing.T) {
	bookkeeping := []Event{
		{Kind: "routing.decision"},
		{Kind: "agent.prompt.sent"},
		{Kind: "model.selection"},
		{Kind: "session.cleanup"},
		{Kind: "session.policy.applied"},
	}
	if turnEvidence("", bookkeeping) {
		t.Fatal("operational events were counted as evidence of a turn")
	}
	if !turnEvidence("", []Event{{Kind: "tool.call.requested"}}) {
		t.Fatal("a requested tool call is evidence of a turn")
	}
	if !turnEvidence("   ", []Event{{Kind: "agent.message.progress"}}) {
		t.Fatal("a message event is evidence of a turn")
	}
	if !turnEvidence("answer", nil) {
		t.Fatal("output is evidence of a turn")
	}
}

// TestWhitespaceOutputIsNotDeliverable closes the degenerate case: a peer that
// only ever emitted formatting characters produced nothing a consumer can use,
// and must not pass as a completed run just because its output was non-empty.
func TestWhitespaceOutputIsNotDeliverable(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)
	completed, err := store.Complete(run.ID, "   \n\t ", "")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.Status != StatusFailed {
		t.Fatalf("whitespace-only output was reported as %q", completed.Status)
	}
	if strings.TrimSpace(completed.Output) != "" {
		t.Fatalf("expected the stored output to be normalised, got %q", completed.Output)
	}
}

// TestCompleteKeepsASuccessfulRunSuccessful is the counterweight: the rule that
// fails an empty turn must not touch a run that did produce something, and the
// terminal transition must stay the single one it always was.
func TestCompleteKeepsASuccessfulRunSuccessful(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)
	completed, err := store.Complete(run.ID, "delivered", "end_turn")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.Status != StatusCompleted || completed.StopReason != "end_turn" {
		t.Fatalf("successful run changed: %+v", completed)
	}
	if completed.OutputRef == "" || completed.OutputDigest == "" {
		t.Fatalf("successful run lost its output reference: %+v", completed)
	}
	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	terminal := findEvent(events, "run.completed")
	if terminal == nil {
		t.Fatalf("expected run.completed, got %+v", events)
	}
	if terminal.StopReason != "end_turn" {
		t.Fatalf("terminal event lost the reported stop reason: %+v", terminal)
	}
	if hasEventKind(events, "run.failed") {
		t.Fatalf("successful run also recorded a failure: %+v", events)
	}
	// A retried transition must remain a no-op.
	repeated, err := store.Complete(run.ID, "", "")
	if err != nil {
		t.Fatalf("repeat complete: %v", err)
	}
	if repeated.Status != StatusCompleted || repeated.Output != "delivered" {
		t.Fatalf("retried transition rewrote the terminal state: %+v", repeated)
	}
}

// TestTraceCarriesTheReportedStopReason proves the distinction survives to the
// exported projection a consumer reads, in both the run and the outcome.
func TestTraceCarriesTheReportedStopReason(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)
	if _, err := store.Complete(run.ID, "", ""); err != nil {
		t.Fatalf("complete empty turn: %v", err)
	}
	trace, found, err := store.Trace(run.ID)
	if err != nil || !found {
		t.Fatalf("trace found=%v err=%v", found, err)
	}
	if trace.Outcome.StopReason != StopReasonReportedNone {
		t.Fatalf("outcome stop reason = %q, want %q", trace.Outcome.StopReason, StopReasonReportedNone)
	}
	if trace.Run.StopReason != StopReasonReportedNone {
		t.Fatalf("trace run stop reason = %q, want %q", trace.Run.StopReason, StopReasonReportedNone)
	}
	if IsReportedStopReason(trace.Outcome.StopReason) {
		t.Fatal("an absent stop reason was reported as one the peer stated")
	}
}

func hasEventKind(events []Event, kind string) bool {
	return findEvent(events, kind) != nil
}

func findEvent(events []Event, kind string) *Event {
	for i := range events {
		if events[i].Kind == kind {
			return &events[i]
		}
	}
	return nil
}
