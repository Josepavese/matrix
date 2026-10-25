package runapi

import (
	"strings"
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

// TestExplainReportsDeniedPermissionNonDeliveryAsTerminal is the regression
// guard for the reproduced trace: a denied tool permission ended the turn and no
// answer arrived. Explain must name the terminal failure in both languages, must
// not call the outcome uncertain, and must not confirm the prompt by a result
// that never arrived.
func TestExplainReportsDeniedPermissionNonDeliveryAsTerminal(t *testing.T) {
	events := []runtrace.Event{
		{Kind: runtrace.KindPermissionRequested},
		{Kind: runtrace.KindPermissionResolved, Outputs: map[string]interface{}{"decision": "denied"}},
		{Kind: runtrace.KindToolCallRequested},
		{Kind: runtrace.KindToolResultReceived, Status: runtrace.StatusFailed},
		{Kind: "run.failed", Metadata: map[string]interface{}{"failure_code": runtrace.FailureCodeProviderToolPermissionDenied}},
	}
	for _, lang := range []string{"en", "it"} {
		t.Run(lang, func(t *testing.T) {
			explained := explainRun(runtrace.Run{ID: "r5", Status: runtrace.StatusFailed, StopReason: "cancelled"}, events, lang)
			if explained.FailureCode != runtrace.FailureCodeProviderToolPermissionDenied {
				t.Fatalf("failure code = %q", explained.FailureCode)
			}
			want := runExplanations[lang][runtrace.FailureCodeProviderToolPermissionDenied]
			if explained.Cause != want.cause || explained.NextAction != want.action || explained.Uncertain != want.uncertain {
				t.Fatalf("explanation = %+v, want %+v", explained, want)
			}
			if explained.Uncertain {
				t.Fatal("a terminal denied-permission non-delivery must not be reported as uncertain")
			}
			if explained.PromptReceipt == "confirmed_by_result" {
				t.Fatalf("a prompt with no delivered answer was receipted as %q", explained.PromptReceipt)
			}
			terminal := "terminal"
			if lang == "it" {
				terminal = "terminale"
			}
			if !strings.Contains(explained.NextAction, terminal) {
				t.Fatalf("next action must say the turn is %s, got %q", terminal, explained.NextAction)
			}
			if explained.StopReason != "cancelled" {
				t.Fatalf("the provider's own stop reason was replaced: %q", explained.StopReason)
			}
		})
	}
}

// TestExplainDoesNotConfirmALegacyCompletedDeniedNonDelivery covers records
// written before the completion rule learned this case: they still say
// "completed", but their trace shows a denied permission and no answer. Explain
// reads the trace, names the failure, and never confirms the prompt.
func TestExplainDoesNotConfirmALegacyCompletedDeniedNonDelivery(t *testing.T) {
	explained := explainRun(
		runtrace.Run{ID: "r6", Status: runtrace.StatusCompleted, StopReason: "cancelled"},
		[]runtrace.Event{
			{Kind: runtrace.KindPermissionResolved, Outputs: map[string]interface{}{"decision": "denied"}},
			{Kind: runtrace.KindToolResultReceived, Status: runtrace.StatusFailed},
			{Kind: "run.completed"},
		},
		"en",
	)
	if explained.PromptReceipt == "confirmed_by_result" {
		t.Fatalf("a denied non-delivery was receipted as %q", explained.PromptReceipt)
	}
	if explained.FailureCode != runtrace.FailureCodeProviderToolPermissionDenied {
		t.Fatalf("failure code = %q", explained.FailureCode)
	}
	if explained.Cause == runExplanations["en"]["completed"].cause {
		t.Fatalf("a denied non-delivery was explained as a completion: %+v", explained)
	}
	if explained.Uncertain {
		t.Fatal("the trace evidences the failure; it must not be reported as uncertain")
	}
}

// TestExplainKeepsAnsweredDeniedTurnCompleted is the counterweight: a denial
// that was followed by a delivered answer stays a completed, answered run.
func TestExplainKeepsAnsweredDeniedTurnCompleted(t *testing.T) {
	explained := explainRun(
		runtrace.Run{ID: "r7", Status: runtrace.StatusCompleted, StopReason: "end_turn"},
		[]runtrace.Event{
			{Kind: runtrace.KindPermissionResolved, Outputs: map[string]interface{}{"decision": "denied"}},
			{Kind: "agent.message.final"},
			{Kind: "run.completed"},
		},
		"en",
	)
	if explained.FailureCode != "" {
		t.Fatalf("an answered turn gained a failure code: %q", explained.FailureCode)
	}
	if explained.Cause != runExplanations["en"]["completed"].cause {
		t.Fatalf("an answered turn was explained as %q", explained.Cause)
	}
	if explained.PromptReceipt != "confirmed_by_result" {
		t.Fatalf("an answered turn was receipted as %q", explained.PromptReceipt)
	}
	if explained.Uncertain {
		t.Fatal("a completed answered turn must not be uncertain")
	}
}

// TestExplainTreatsApprovedPermissionNormally keeps the approved permission path
// untouched.
func TestExplainTreatsApprovedPermissionNormally(t *testing.T) {
	explained := explainRun(
		runtrace.Run{ID: "r8", Status: runtrace.StatusCompleted, StopReason: "end_turn"},
		[]runtrace.Event{
			{Kind: runtrace.KindPermissionResolved, Outputs: map[string]interface{}{"decision": "approved"}},
			{Kind: "agent.message.final"},
			{Kind: "run.completed"},
		},
		"en",
	)
	if explained.FailureCode != "" || explained.Cause != runExplanations["en"]["completed"].cause {
		t.Fatalf("an approved permission changed the explanation: %+v", explained)
	}
	if explained.PromptReceipt != "confirmed_by_result" {
		t.Fatalf("an answered run was receipted as %q", explained.PromptReceipt)
	}
}

// TestExplainKeepsCancelledWithoutDenialUnchanged proves the narrowed scope: a
// cancellation with no denied permission keeps its own explanation.
func TestExplainKeepsCancelledWithoutDenialUnchanged(t *testing.T) {
	explained := explainRun(
		runtrace.Run{ID: "r9", Status: runtrace.StatusCancelled, StopReason: "cancelled"},
		[]runtrace.Event{
			{Kind: runtrace.KindToolResultReceived, Status: runtrace.StatusFailed},
			{Kind: "run.cancelled"},
		},
		"en",
	)
	if explained.FailureCode != "" {
		t.Fatalf("a plain cancellation gained a failure code: %q", explained.FailureCode)
	}
	if explained.Cause != runExplanations["en"]["cancelled"].cause || !explained.Uncertain {
		t.Fatalf("a plain cancellation changed meaning: %+v", explained)
	}
	if explained.PromptReceipt == "confirmed_by_result" {
		t.Fatalf("a cancelled run was receipted as %q", explained.PromptReceipt)
	}
}
