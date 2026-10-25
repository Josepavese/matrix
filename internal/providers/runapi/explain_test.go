package runapi

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

func TestExplainRunDistinguishesRemoteUncertaintyAndProviderWorkspaceRefusal(t *testing.T) {
	unknown := explainRun(runtrace.Run{ID: "r1", Status: runtrace.StatusUnknown}, nil, "it")
	if !unknown.Uncertain || unknown.PromptReceipt != "unverified" || unknown.NextAction == "" {
		t.Fatalf("unknown outcome explanation: %+v", unknown)
	}
	refused := explainRun(runtrace.Run{ID: "r2", Status: runtrace.StatusFailed}, []runtrace.Event{{Kind: "provider.preflight.failed", ProtocolMethod: "session/new", Metadata: map[string]interface{}{"code": "provider_workspace_rejected"}}}, "en")
	if refused.FailureCode != "provider_workspace_rejected" || refused.Phase != "session/new" || refused.NextAction == unknown.NextAction {
		t.Fatalf("workspace explanation: %+v", refused)
	}
}

// TestExplainFailureCodeKeepsEventStreamPrecedence pins the failure-code rule the
// extraction moved into explainFailureCode: a typed provider failure overwrites an
// already-recorded failure, while a recorded failure only fills an empty code.
func TestExplainFailureCodeKeepsEventStreamPrecedence(t *testing.T) {
	typedAfterRecorded := explainRun(
		runtrace.Run{ID: "r3", Status: runtrace.StatusFailed},
		[]runtrace.Event{
			{Kind: "run.failed", Metadata: map[string]interface{}{"failure_code": "run_no_turn_evidence"}},
			{Kind: "provider.runtime.failed", ProtocolMethod: "session/prompt", Metadata: map[string]interface{}{"code": "provider_model_unavailable"}},
		},
		"en",
	)
	if typedAfterRecorded.FailureCode != "provider_model_unavailable" || typedAfterRecorded.Phase != "session/prompt" {
		t.Fatalf("a typed provider failure must win over an earlier recorded failure: %+v", typedAfterRecorded)
	}

	recordedAfterTyped := explainRun(
		runtrace.Run{ID: "r4", Status: runtrace.StatusFailed},
		[]runtrace.Event{
			{Kind: "provider.runtime.failed", ProtocolMethod: "session/prompt", Metadata: map[string]interface{}{"code": "provider_model_unavailable"}},
			{Kind: "run.failed", Metadata: map[string]interface{}{"failure_code": "run_no_turn_evidence"}},
		},
		"en",
	)
	if recordedAfterTyped.FailureCode != "provider_model_unavailable" || recordedAfterTyped.Phase != "session/prompt" {
		t.Fatalf("a recorded failure must not overwrite a typed provider failure: %+v", recordedAfterTyped)
	}
}
