package frontendevents

import "testing"

// TestNormalizePermissionProjectsTheStructuredDiagnosis pins the public
// projection contract: a consumer must be able to read what was requested and
// why it was resolved from the request inputs and the resolution outputs alone,
// without parsing the permission payload.
func TestNormalizePermissionProjectsTheStructuredDiagnosis(t *testing.T) {
	metadata := map[string]interface{}{
		"protocol_method":   "session/request_permission",
		"decision":          "denied",
		"option_id":         "reject-1",
		"approval_mode":     "manual_or_policy",
		"operation":         "pwd",
		"operation_state":   PermissionOperationKnown,
		"tool_call_id":      "call-7",
		"session_id":        "session-3",
		"denial_reason":     PermissionDenialTrustModeOff,
		"provider_response": PermissionResponseSelected,
	}
	permission := NormalizePermission("run-1", "deny the pwd call", metadata)

	for key, want := range map[string]string{
		"operation":       "pwd",
		"operation_state": PermissionOperationKnown,
		"tool_call_id":    "call-7",
		"session_id":      "session-3",
	} {
		if got := permission.RequestInputs[key]; got != want {
			t.Fatalf("RequestInputs[%q] = %v, want %q", key, got, want)
		}
	}
	for key, want := range map[string]string{
		"decision":          "denied",
		"option_id":         "reject-1",
		"denial_reason":     PermissionDenialTrustModeOff,
		"provider_response": PermissionResponseSelected,
	} {
		if got := permission.ResolutionOutputs[key]; got != want {
			t.Fatalf("ResolutionOutputs[%q] = %v, want %q", key, got, want)
		}
	}
}

// TestNormalizePermissionAlwaysStatesTheOperationState keeps the field that the
// consumer was missing: an absent operation is reported as unknown, never
// silently omitted, and no payload heuristic overrides an explicit unknown.
func TestNormalizePermissionAlwaysStatesTheOperationState(t *testing.T) {
	unresolved := NormalizePermission("run-1", "Allow running rm -rf?", nil)
	if unresolved.RequestInputs["operation_state"] != PermissionOperationUnknown {
		t.Fatalf("an unidentifiable permission must state unknown, got %#v", unresolved.RequestInputs)
	}
	if _, ok := unresolved.RequestInputs["operation"]; ok {
		t.Fatalf("an unknown operation must not be fabricated, got %#v", unresolved.RequestInputs)
	}

	// A producer that inspected the request and declared the operation unknown
	// wins over the payload fallback, even when the payload happens to contain a
	// command-shaped string.
	declaredUnknown := NormalizePermission("run-1", `{"command":"pwd"}`, map[string]interface{}{
		"operation_state": PermissionOperationUnknown,
	})
	if declaredUnknown.RequestInputs["operation_state"] != PermissionOperationUnknown {
		t.Fatalf("declared unknown state was overridden: %#v", declaredUnknown.RequestInputs)
	}
	if _, ok := declaredUnknown.RequestInputs["operation"]; ok {
		t.Fatalf("declared unknown state must not gain an operation: %#v", declaredUnknown.RequestInputs)
	}

	// A declared known state without an operation is downgraded instead of
	// trusted, so the two fields can never disagree.
	inconsistent := NormalizePermission("run-1", "", map[string]interface{}{
		"operation_state": PermissionOperationKnown,
	})
	if inconsistent.RequestInputs["operation_state"] != PermissionOperationUnknown {
		t.Fatalf("inconsistent declared state must be downgraded, got %#v", inconsistent.RequestInputs)
	}
}

// TestNormalizePermissionKeepsLegacyProducersWorking keeps the historical
// extraction for permission updates that predate the structured state.
func TestNormalizePermissionKeepsLegacyProducersWorking(t *testing.T) {
	legacy := NormalizePermission("run-1", `{"command":"go test ./..."}`, nil)
	if legacy.RequestInputs["operation"] != "go" || legacy.RequestInputs["operation_state"] != PermissionOperationKnown {
		t.Fatalf("legacy command extraction lost: %#v", legacy.RequestInputs)
	}
}
