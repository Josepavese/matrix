package a2aclient

import "github.com/jose/matrix-v2/internal/middleware"

func (c *a2aConversationClient) SessionCapabilities() middleware.ConversationSessionCapabilities {
	return middleware.ConversationSessionCapabilities{
		List:   true,
		Load:   true,
		Cancel: true,
		Delete: true,
		Details: map[string]middleware.CapabilityDescriptor{
			"list":   a2aCapability("list", true, "stable", "a2a_tasks/list"),
			"load":   a2aCapability("load", true, "stable", "a2a_task_get"),
			"cancel": a2aCapability("cancel", true, "stable", "a2a_tasks/cancel"),
			"delete": a2aCapability("delete", true, "stable", "a2a_task_delete"),
			"close":  a2aCapability("close", false, "unsupported", "a2a_no_close_mapping"),
			"resume": a2aCapability("resume", false, "unsupported", "a2a_task_state_mapping"),
			"fork":   a2aCapability("fork", false, "unsupported", "a2a_no_fork_mapping"),
		},
	}
}

func a2aCapability(name string, supported bool, stability, source string) middleware.CapabilityDescriptor {
	status := "unsupported"
	if supported {
		status = "supported"
	}
	desc := middleware.CapabilityDescriptor{Name: name, Supported: supported, Status: status, Stability: stability, Source: source}
	if name == "fork" {
		desc.ActiveParentSafe = boolPtr(false)
		desc.RequiresIdleParent = boolPtr(false)
		desc.ArtifactTurn = boolPtr(false)
	}
	return desc
}

func boolPtr(value bool) *bool {
	return &value
}
