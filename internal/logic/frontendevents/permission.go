package frontendevents

// The permission projection pins a small vocabulary. An operation is either
// known or explicitly unknown, a denied approval names why Matrix refused it,
// and provider_response says what was actually returned to the peer. These are
// projection values, not user-facing prose: changing one is a contract change.
const (
	PermissionOperationKnown   = "known"
	PermissionOperationUnknown = "unknown"

	PermissionDenialTrustModeOff  = "trust_mode_off"
	PermissionDenialNoAllowOption = "no_allow_option"

	PermissionResponseSelected  = "selected"
	PermissionResponseCancelled = "cancelled"
)

type PermissionEvent struct {
	ID                string
	ProtocolMethod    string
	RequestSummary    string
	RequestInputs     map[string]interface{}
	ResolutionSummary string
	ResolutionOutputs map[string]interface{}
	ApprovalMode      string
}

func NormalizePermission(runID, content string, metadata map[string]interface{}) PermissionEvent {
	decision := FirstNonEmpty(StringValue(metadata, "decision"), "unknown")
	optionID := StringValue(metadata, "option_id")
	return PermissionEvent{
		ID:                StablePermissionID(runID, content),
		ProtocolMethod:    FirstNonEmpty(StringValue(metadata, "protocol_method"), "session/request_permission"),
		RequestSummary:    permissionSummary(content),
		RequestInputs:     permissionInputs(content, metadata),
		ResolutionSummary: permissionResolutionSummary(decision, optionID),
		ResolutionOutputs: permissionOutputs(decision, optionID, metadata),
		ApprovalMode:      StringValue(metadata, "approval_mode"),
	}
}

func permissionSummary(content string) string {
	if path := FirstPath(content, nil); path != "" {
		return "Permission requested for " + path
	}
	return "Permission requested"
}

func permissionResolutionSummary(decision, optionID string) string {
	if optionID == "" {
		return "Permission " + decision
	}
	return "Permission " + decision + " with " + optionID
}

func permissionInputs(content string, metadata map[string]interface{}) map[string]interface{} {
	inputs := map[string]interface{}{}
	if path := FirstPath(content, metadata); path != "" {
		inputs["path"] = path
	}
	operation, operationState := permissionOperation(content, metadata)
	if operation != "" {
		inputs["operation"] = operation
	}
	// operation_state is always present: a capture that cannot identify the
	// operation must say unknown rather than leave the field silently absent.
	inputs["operation_state"] = operationState
	if toolCallID := StringValue(metadata, "tool_call_id"); toolCallID != "" {
		inputs["tool_call_id"] = toolCallID
	}
	if sessionID := StringValue(metadata, "session_id"); sessionID != "" {
		inputs["session_id"] = sessionID
	}
	if options, ok := metadata["options"]; ok && options != nil {
		inputs["options"] = options
	}
	return inputs
}

// permissionOperation resolves the operation identity of a permission request.
// When the producer declares operation_state that declaration is authoritative:
// an explicit "unknown" is never replaced by a guess extracted from the payload,
// and a "known" state without an operation is downgraded rather than trusted.
// Producers that predate the state keep the historical extraction from the
// structured metadata and JSON payload.
func permissionOperation(content string, metadata map[string]interface{}) (string, string) {
	switch StringValue(metadata, "operation_state") {
	case PermissionOperationKnown:
		if operation := StringValue(metadata, "operation"); operation != "" {
			return operation, PermissionOperationKnown
		}
		return "", PermissionOperationUnknown
	case PermissionOperationUnknown:
		return "", PermissionOperationUnknown
	}
	if operation := ToolOperation(content, metadata); operation != "" {
		return operation, PermissionOperationKnown
	}
	return "", PermissionOperationUnknown
}

func permissionOutputs(decision, optionID string, metadata map[string]interface{}) map[string]interface{} {
	outputs := map[string]interface{}{"decision": decision}
	if optionID != "" {
		outputs["option_id"] = optionID
	}
	if providerResponse := StringValue(metadata, "provider_response"); providerResponse != "" {
		outputs["provider_response"] = providerResponse
	}
	if denialReason := StringValue(metadata, "denial_reason"); denialReason != "" {
		outputs["denial_reason"] = denialReason
	}
	return outputs
}
