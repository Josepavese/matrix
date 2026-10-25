package runtrace

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

// completeWithEvents appends the given events and drives a real terminal
// transition, returning the stored run and every event the store now holds.
func completeWithEvents(t *testing.T, events []Event, output, stopReason string) (Run, []Event) {
	t.Helper()
	store := NewStore(memstore.New())
	run := startTestRun(t, store)
	for _, event := range events {
		event.RunID = run.ID
		if _, err := store.AppendEvent(event); err != nil {
			t.Fatalf("append event: %v", err)
		}
	}
	completed, err := store.Complete(run.ID, output, stopReason)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	loaded, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	return completed, loaded
}

func deniedPermissionResolvedEvent() Event {
	return Event{Kind: KindPermissionResolved, Outputs: map[string]interface{}{"decision": "denied"}}
}

func approvedPermissionResolvedEvent() Event {
	return Event{Kind: KindPermissionResolved, Outputs: map[string]interface{}{"decision": "approved"}}
}

// TestDeniedPermissionNonDeliveryFailsTheRun is the regression guard for the
// reproduced trace: permission requested, permission resolved denied, the tool
// result failed, the provider reported stop_reason=cancelled and delivered no
// answer. Tool events are evidence the peer acted, not evidence it answered, so
// the turn must be reported as a terminal failure instead of "completed".
func TestDeniedPermissionNonDeliveryFailsTheRun(t *testing.T) {
	run, events := completeWithEvents(t, []Event{
		{Kind: KindPermissionRequested},
		deniedPermissionResolvedEvent(),
		{Kind: KindToolCallRequested},
		{Kind: KindToolResultReceived, Status: StatusFailed},
		{Kind: KindTurnStopReason, Metadata: map[string]interface{}{StopReasonMetadataKey: "cancelled"}},
	}, "", "cancelled")

	if run.Status != StatusFailed {
		t.Fatalf("a denied-permission non-delivery was reported as %q", run.Status)
	}
	if run.StopReason != "cancelled" {
		t.Fatalf("the provider's own stop reason was replaced: %q", run.StopReason)
	}
	if !strings.Contains(run.Error, "denied") || !strings.Contains(run.Error, "without delivering an answer") {
		t.Fatalf("the failure must state the denied non-delivery, got %q", run.Error)
	}
	if run.Output != "" || run.OutputRef != "" {
		t.Fatalf("a non-delivery must not carry an output: %#v", run)
	}
	if hasEventKind(events, "run.completed") {
		t.Fatalf("a denied-permission non-delivery emitted run.completed: %+v", events)
	}
	if hasEventKind(events, "agent.message.final") {
		t.Fatalf("a denied-permission non-delivery emitted a final message: %+v", events)
	}
	failed := findEvent(events, "run.failed")
	if failed == nil {
		t.Fatalf("expected a run.failed event, got %+v", events)
	}
	if failed.Metadata["failure_code"] != FailureCodeProviderToolPermissionDenied {
		t.Fatalf("failure_code = %v, want %q", failed.Metadata["failure_code"], FailureCodeProviderToolPermissionDenied)
	}
	if failed.StopReason != "cancelled" {
		t.Fatalf("terminal event lost the reported stop reason: %+v", failed)
	}
}

// TestDeniedPermissionWithAMessageStillCompletes is the counterweight: when the
// peer does deliver an answer after the denial, the turn completed and the
// denial verdict must not turn it into a failure.
func TestDeniedPermissionWithAMessageStillCompletes(t *testing.T) {
	run, events := completeWithEvents(t, []Event{
		deniedPermissionResolvedEvent(),
		{Kind: "agent.message.final"},
	}, "", "end_turn")
	if run.Status != StatusCompleted {
		t.Fatalf("a denied turn that delivered a message was reported as %q", run.Status)
	}
	if hasEventKind(events, "run.failed") {
		t.Fatalf("an answered turn recorded a failure: %+v", events)
	}

	// Output alone is also an answer, even without message events.
	delivered, _ := completeWithEvents(t, []Event{
		deniedPermissionResolvedEvent(),
		{Kind: KindToolResultReceived, Status: StatusFailed},
	}, "delivered answer", "end_turn")
	if delivered.Status != StatusCompleted {
		t.Fatalf("a denied turn that delivered output was reported as %q", delivered.Status)
	}
}

// TestCancelledTurnWithoutDenialKeepsCurrentSemantics proves the new rule is
// scoped to denied permissions: a cancelled turn with tool evidence keeps
// completing, and an empty cancelled turn keeps failing with the generic code.
func TestCancelledTurnWithoutDenialKeepsCurrentSemantics(t *testing.T) {
	withTools, events := completeWithEvents(t, []Event{
		{Kind: KindToolCallRequested},
		{Kind: KindToolResultReceived, Status: StatusFailed},
	}, "", "cancelled")
	if withTools.Status != StatusCompleted || withTools.StopReason != "cancelled" {
		t.Fatalf("tool evidence without a denial changed meaning: %+v", withTools)
	}
	if hasEventKind(events, "run.failed") {
		t.Fatalf("a cancelled turn with tool evidence recorded a failure: %+v", events)
	}

	empty, events := completeWithEvents(t, nil, "", "cancelled")
	if empty.Status != StatusFailed {
		t.Fatalf("an empty cancelled turn was reported as %q", empty.Status)
	}
	failed := findEvent(events, "run.failed")
	if failed == nil || failed.Metadata["failure_code"] != completionFailureCode {
		t.Fatalf("an empty cancelled turn must keep the generic code, got %+v", events)
	}
}

// TestApprovedPermissionStillCompletes keeps the approved path untouched: an
// approval is not a denial, and a turn that answered after it is a success.
func TestApprovedPermissionStillCompletes(t *testing.T) {
	run, events := completeWithEvents(t, []Event{
		{Kind: KindPermissionRequested},
		approvedPermissionResolvedEvent(),
		{Kind: "agent.message.final"},
	}, "answer", "end_turn")
	if run.Status != StatusCompleted {
		t.Fatalf("an approved permission turn was reported as %q", run.Status)
	}
	if hasEventKind(events, "run.failed") {
		t.Fatalf("an approved permission turn recorded a failure: %+v", events)
	}
}

// TestDeniedDecisionOutsideOutputsDoesNotTrigger keeps the detector structural:
// only the resolution's structured decision field counts, so bookkeeping that
// merely mentions a denial cannot change a run's verdict.
func TestDeniedDecisionOutsideOutputsDoesNotTrigger(t *testing.T) {
	run, events := completeWithEvents(t, []Event{
		{Kind: KindPermissionResolved, Inputs: map[string]interface{}{"decision": "denied"}},
		{Kind: KindToolResultReceived},
	}, "", "end_turn")
	if run.Status != StatusCompleted {
		t.Fatalf("a decision outside outputs changed the verdict: %q", run.Status)
	}
	if hasEventKind(events, "run.failed") {
		t.Fatalf("a decision outside outputs recorded a failure: %+v", events)
	}
}
