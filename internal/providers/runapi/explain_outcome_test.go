package runapi

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

// TestExplainDoesNotConfirmAPromptWithoutATurnResult is the regression guard for
// the observation recorded against issue 3: "/explain reported
// prompt_receipt=confirmed_by_result" for a run whose trace had no final message,
// no tool call and no output. The terminal event says the run stopped; it does
// not say the peer answered.
//
// The `completed` case is the one that was actually observed, and it is kept even
// though the run lifecycle no longer produces it: records written before the rule
// existed are still read by consumers, and a receipt that flips to "confirmed"
// because a terminal event exists is exactly what made an empty run look answered.
func TestExplainDoesNotConfirmAPromptWithoutATurnResult(t *testing.T) {
	completed := explainRun(
		runtrace.Run{ID: "r0", Status: runtrace.StatusCompleted, StopReason: runtrace.StopReasonReportedNone},
		[]runtrace.Event{
			{Kind: "run.started"},
			{Kind: "agent.prompt.sent"},
			{Kind: "model.selection"},
			{Kind: "run.completed"},
		},
		"en",
	)
	if completed.PromptReceipt != "unverified" {
		t.Fatalf("a completed run with no message, tool call or output was receipted as %q", completed.PromptReceipt)
	}

	empty := explainRun(
		runtrace.Run{ID: "r1", Status: runtrace.StatusFailed, StopReason: runtrace.StopReasonReportedNone},
		[]runtrace.Event{
			{Kind: "run.started"},
			{Kind: "agent.prompt.sent"},
			{Kind: "model.selection"},
			{Kind: "run.failed", Metadata: map[string]interface{}{"failure_code": "run_no_turn_evidence"}},
		},
		"en",
	)
	if empty.PromptReceipt != "unverified" {
		t.Fatalf("a run that produced nothing was receipted as %q", empty.PromptReceipt)
	}
	if empty.FailureCode != "run_no_turn_evidence" || empty.NextAction == "" {
		t.Fatalf("expected the empty turn to be explained, got %+v", empty)
	}
	if empty.StopReason != runtrace.StopReasonReportedNone {
		t.Fatalf("explain invented a stop reason: %q", empty.StopReason)
	}
}

// TestExplainConfirmsAPromptWithATurnResult is the counterweight: a run whose
// peer did produce something still reports a confirmed receipt, so the rule
// distinguishes the two cases instead of making every run look unverified.
func TestExplainConfirmsAPromptWithATurnResult(t *testing.T) {
	answered := explainRun(
		runtrace.Run{ID: "r2", Status: runtrace.StatusCompleted, StopReason: "end_turn"},
		[]runtrace.Event{
			{Kind: "agent.prompt.sent"},
			{Kind: "agent.message.final"},
			{Kind: "run.completed"},
		},
		"en",
	)
	if answered.PromptReceipt != "confirmed_by_result" {
		t.Fatalf("a run with a final message was reported as %q", answered.PromptReceipt)
	}
	if answered.StopReason != "end_turn" {
		t.Fatalf("the provider's own stop reason was replaced: %q", answered.StopReason)
	}
	toolOnly := explainRun(
		runtrace.Run{ID: "r3", Status: runtrace.StatusCompleted, StopReason: runtrace.StopReasonReportedNone},
		[]runtrace.Event{
			{Kind: "tool.call.requested"},
			{Kind: "tool.result.received"},
			{Kind: "run.completed"},
		},
		"en",
	)
	if toolOnly.PromptReceipt != "confirmed_by_result" {
		t.Fatalf("a run that ran tools was reported as %q", toolOnly.PromptReceipt)
	}
}

// TestExplainReceiptIgnoresMatrixBookkeeping proves the receipt rule cannot be
// satisfied by the events every run has, which is what made an empty run look
// confirmed in the first place.
func TestExplainReceiptIgnoresMatrixBookkeeping(t *testing.T) {
	bookkeeping := []runtrace.Event{
		{Kind: "routing.decision"},
		{Kind: "agent.prompt.sent"},
		{Kind: "model.selection"},
		{Kind: "session.cleanup"},
		{Kind: "session.policy.applied"},
		{Kind: "run.completed"},
	}
	if hasTurnEvidence(bookkeeping) {
		t.Fatal("operational events were counted as a turn result")
	}
	explained := explainRun(runtrace.Run{ID: "r4", Status: runtrace.StatusCompleted}, bookkeeping, "en")
	if explained.PromptReceipt == "confirmed_by_result" {
		t.Fatalf("bookkeeping alone confirmed the prompt: %+v", explained)
	}
}
