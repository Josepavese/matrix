package frontendevents

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
		ResolutionOutputs: permissionOutputs(decision, optionID),
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
	if options, ok := metadata["options"]; ok && options != nil {
		inputs["options"] = options
	}
	if len(inputs) == 0 {
		return nil
	}
	return inputs
}

func permissionOutputs(decision, optionID string) map[string]interface{} {
	outputs := map[string]interface{}{"decision": decision}
	if optionID != "" {
		outputs["option_id"] = optionID
	}
	return outputs
}
