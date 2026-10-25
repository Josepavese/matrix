package agents

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Josepavese/matrix/pkg/zedacp"
)

// decidePermission drives the real request handler and returns the structured
// metadata it reported for the decision.
func decidePermission(t *testing.T, trusted bool, version int, params string) map[string]interface{} {
	t.Helper()
	notifier := &observerTestNotifier{}
	handler := newConfigurableRequestHandler(func() bool { return trusted })
	handler.protocolVersion.Store(int32(version))
	handler.WithNotifier(notifier)
	if _, err := handler.HandleRequest(context.Background(), "session/request_permission", json.RawMessage(params)); err != nil {
		t.Fatalf("permission request: %v", err)
	}
	if len(notifier.updates) != 1 {
		t.Fatalf("expected one decision report, got %d", len(notifier.updates))
	}
	return notifier.updates[0].Metadata
}

// TestPermissionDenialDiagnosticsV1StructuredSources pins that version 1's
// structured fields name the operation and correlate the denial, while the
// human-facing title is never used as the operation.
func TestPermissionDenialDiagnosticsV1StructuredSources(t *testing.T) {
	meta := decidePermission(t, true, supportedACPProtocolVersion, `{
		"sessionId": "session-1",
		"toolCall": {"toolCallId": "call-1", "title": "Run pwd", "kind": "execute", "rawInput": {"command": "pwd"}},
		"options": [
			{"optionId": "allow-once", "name": "Allow", "kind": "allow_once"},
			{"optionId": "reject-once", "name": "Reject", "kind": "reject_once"}
		]}`)
	if meta["operation"] != "pwd" || meta["operation_state"] != "known" {
		t.Fatalf("the structured raw input must name the operation, got %#v", meta)
	}
	if meta["tool_call_id"] != "call-1" || meta["session_id"] != "session-1" {
		t.Fatalf("the request must be correlated, got %#v", meta)
	}
	if meta["decision"] != "approved" || meta["provider_response"] != "selected" {
		t.Fatalf("an auto-approved request must say so, got %#v", meta)
	}
	if _, ok := meta["denial_reason"]; ok {
		t.Fatalf("an approval has no denial reason, got %#v", meta)
	}
	// Only the operation token is carried: the raw input itself stays out of the
	// diagnosis, so provider arguments cannot leak into the trace.
	if _, ok := meta["raw_input"]; ok {
		t.Fatalf("the raw tool input must not be exported, got %#v", meta)
	}
}

// TestPermissionDenialDiagnosticsV1UnknownOperation keeps the honest case: a
// version 1 request with only a human title has no structured operation, so the
// diagnosis says unknown instead of inventing an operation from prose.
func TestPermissionDenialDiagnosticsV1UnknownOperation(t *testing.T) {
	meta := decidePermission(t, false, supportedACPProtocolVersion, `{
		"sessionId": "session-2",
		"toolCall": {"toolCallId": "call-2", "title": "pwd"},
		"options": [{"optionId": "allow-once", "name": "Allow", "kind": "allow_once"}]}`)
	if _, ok := meta["operation"]; ok {
		t.Fatalf("a title must never become an operation, got %#v", meta)
	}
	if meta["operation_state"] != "unknown" {
		t.Fatalf("an unidentifiable operation must be explicit, got %#v", meta)
	}
	if meta["tool_call_id"] != "call-2" || meta["session_id"] != "session-2" {
		t.Fatalf("correlation ids must survive an unknown operation, got %#v", meta)
	}
	if meta["denial_reason"] != "trust_mode_off" {
		t.Fatalf("a trust-mode refusal must name its reason, got %#v", meta)
	}
	if meta["provider_response"] != "cancelled" {
		t.Fatalf("a denial without a reject option must report the cancelled outcome, got %#v", meta)
	}
}

// TestPermissionDenialDiagnosticsV1KindIsACategory keeps the tool kind visible
// when the peer names no exact command: the field is a protocol enum, so it is a
// deterministic category-level operation.
func TestPermissionDenialDiagnosticsV1KindIsACategory(t *testing.T) {
	meta := decidePermission(t, false, supportedACPProtocolVersion, `{
		"sessionId": "session-3",
		"toolCall": {"toolCallId": "call-3", "title": "Write the file", "kind": "edit"},
		"options": [{"optionId": "reject-once", "name": "Reject", "kind": "reject_once"}]}`)
	if meta["operation"] != "edit" || meta["operation_state"] != "known" {
		t.Fatalf("the structured tool kind must become the operation category, got %#v", meta)
	}
	if meta["denial_reason"] != "trust_mode_off" || meta["provider_response"] != "selected" {
		t.Fatalf("the denial must name its reason and provider response, got %#v", meta)
	}
}

// TestPermissionDenialDiagnosticsV2CommandSubject reads version 2's command
// subject: the exact command is structured, so the operation is the executable.
func TestPermissionDenialDiagnosticsV2CommandSubject(t *testing.T) {
	meta := decidePermission(t, true, zedacp.ProtocolVersionV2, `{
		"sessionId": "session-4",
		"title": "Run the test suite?",
		"description": "This runs go test over the workspace.",
		"subject": {"type": "command", "command": "go test ./...", "cwd": "/workspace", "toolCallId": "call-4"},
		"options": [{"optionId": "allow-once", "name": "Allow once", "kind": "allow_once"}]}`)
	if meta["operation"] != "go" || meta["operation_state"] != "known" {
		t.Fatalf("the v2 command subject must name the operation, got %#v", meta)
	}
	if meta["tool_call_id"] != "call-4" || meta["session_id"] != "session-4" {
		t.Fatalf("the v2 command subject must be correlated, got %#v", meta)
	}
	if meta["permission_subject_type"] != "command" {
		t.Fatalf("the subject type must stay visible, got %#v", meta)
	}
	if meta["provider_response"] != "selected" || meta["decision"] != "approved" {
		t.Fatalf("an approval must report the selected outcome, got %#v", meta)
	}
}

// TestPermissionDenialDiagnosticsV2ToolCallSubject reads version 2's tool-call
// subject, whose tool patch carries the correlation id and the tool kind.
func TestPermissionDenialDiagnosticsV2ToolCallSubject(t *testing.T) {
	meta := decidePermission(t, false, zedacp.ProtocolVersionV2, `{
		"sessionId": "session-5",
		"title": "Approve file edit?",
		"subject": {"type": "tool_call", "toolCall": {"toolCallId": "call-5", "kind": "edit"}},
		"options": [{"optionId": "reject-once", "name": "Reject", "kind": "reject_once"}]}`)
	if meta["operation"] != "edit" || meta["operation_state"] != "known" {
		t.Fatalf("the v2 tool-call subject must name the operation, got %#v", meta)
	}
	if meta["tool_call_id"] != "call-5" || meta["session_id"] != "session-5" {
		t.Fatalf("the v2 tool-call subject must be correlated, got %#v", meta)
	}
	if meta["denial_reason"] != "trust_mode_off" || meta["provider_response"] != "selected" {
		t.Fatalf("the denial must name its reason and provider response, got %#v", meta)
	}
}

// TestPermissionDenialDiagnosticsNoAllowOption names the other policy refusal:
// trust mode is on but the peer offered no option Matrix may select.
func TestPermissionDenialDiagnosticsNoAllowOption(t *testing.T) {
	meta := decidePermission(t, true, zedacp.ProtocolVersionV2, `{
		"sessionId": "session-6",
		"title": "Approve reading the file?",
		"subject": {"type": "tool_call", "toolCall": {"toolCallId": "call-6", "kind": "read"}},
		"options": [{"optionId": "reject-once", "name": "Reject", "kind": "reject_once"}]}`)
	if meta["decision"] != "denied" || meta["denial_reason"] != "no_allow_option" {
		t.Fatalf("a refusal without an allow option must name no_allow_option, got %#v", meta)
	}
	if meta["provider_response"] != "selected" {
		t.Fatalf("selecting the offered reject option is a selected provider response, got %#v", meta)
	}
}
