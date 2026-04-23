package agents

import "github.com/Josepavese/matrix/internal/middleware"

func supportsLoadSession(resp *acpInitializeResponse) bool {
	if resp == nil || resp.Capabilities == nil {
		return false
	}
	enabled, _ := resp.Capabilities["loadSession"].(bool)
	return enabled
}

func supportsSessionCapability(resp *acpInitializeResponse, name string) bool {
	if resp == nil || resp.Capabilities == nil {
		return false
	}
	caps, _ := resp.Capabilities["sessionCapabilities"].(map[string]interface{})
	if caps == nil {
		return false
	}
	return capabilityEnabled(caps[name])
}

func capabilityEnabled(raw interface{}) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case map[string]interface{}:
		return value != nil
	default:
		return false
	}
}

func acpSessionCapabilities(resp *acpInitializeResponse) middleware.ConversationSessionCapabilities {
	list := supportsSessionCapability(resp, "list")
	load := supportsLoadSession(resp)
	closeSession := supportsSessionCapability(resp, "close")
	deleteSession := supportsSessionCapability(resp, "delete")
	resume := supportsSessionCapability(resp, "resume")
	fork := supportsSessionCapability(resp, "fork")
	return middleware.ConversationSessionCapabilities{
		List:       list,
		Load:       load,
		Cancel:     true,
		Close:      closeSession,
		Delete:     deleteSession,
		InfoUpdate: list,
		Resume:     resume,
		Fork:       fork,
		Details: map[string]middleware.CapabilityDescriptor{
			"list":        acpCapability("list", list, "stable", "zed_acp_session_list_rfd"),
			"info_update": acpCapability("info_update", list, "stable", "zed_acp_session_info_update"),
			"load":        acpCapability("load", load, "stable", "zed_acp_schema_loadSession"),
			"cancel":      acpCapability("cancel", true, "stable", "zed_acp_schema_session_cancel"),
			"close":       acpCapability("close", closeSession, "preview", "zed_acp_rfd_session_close"),
			"delete":      acpCapability("delete", deleteSession, "draft", "zed_acp_rfd_session_delete"),
			"resume":      acpCapability("resume", resume, "preview", "zed_acp_rfd_session_resume"),
			"fork":        acpForkCapability(fork),
		},
	}
}

func acpCapability(name string, supported bool, stability, source string) middleware.CapabilityDescriptor {
	status := "unsupported"
	if supported {
		status = "supported"
	}
	return middleware.CapabilityDescriptor{
		Name:      name,
		Supported: supported,
		Status:    status,
		Stability: stability,
		Source:    source,
	}
}

func acpForkCapability(supported bool) middleware.CapabilityDescriptor {
	desc := acpCapability("fork", supported, "draft", "zed_acp_rfd_session_fork")
	if supported {
		desc.ActiveParentSafe = boolPtr(true)
		desc.RequiresIdleParent = boolPtr(false)
		desc.ArtifactTurn = boolPtr(true)
	}
	return desc
}

func boolPtr(value bool) *bool {
	return &value
}
