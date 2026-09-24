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
