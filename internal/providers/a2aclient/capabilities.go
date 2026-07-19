package a2aclient

import "github.com/Josepavese/matrix/internal/middleware"

func (c *a2aConversationClient) SessionCapabilities() middleware.ConversationSessionCapabilities {
	return middleware.ConversationSessionCapabilities{
		List:   true,
		Load:   true,
		Cancel: true,
		Delete: false,
		Details: map[string]middleware.CapabilityDescriptor{
			"list":   a2aCapability("list", true, "stable", "A2A ListTasks"),
			"load":   a2aCapability("load", true, "stable", "A2A GetTask"),
			"cancel": a2aCapability("cancel", true, "stable", "A2A CancelTask"),
			"delete": a2aCapability("delete", false, "unsupported", "a2a_no_delete_mapping"),
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

func (c *a2aConversationClient) ProtocolCapabilities() middleware.ProviderCapabilityReport {
	operations := map[string]middleware.CapabilityDescriptor{}
	for _, name := range []string{"SendMessage", "GetTask", "ListTasks", "CancelTask"} {
		operations[name] = a2aCapability(name, true, "stable", "A2A v1.0.1")
	}
	streaming, push, extended := false, false, false
	if c.advertised != nil {
		streaming = c.advertised.Streaming
		push = c.advertised.PushNotifications
		extended = c.advertised.ExtendedAgentCard
	}
	operations["SendStreamingMessage"] = c.advertisedCapability("SendStreamingMessage", streaming)
	operations["SubscribeToTask"] = c.advertisedCapability("SubscribeToTask", streaming)
	for _, name := range []string{"CreateTaskPushConfig", "GetTaskPushConfig", "ListTaskPushConfigs", "DeleteTaskPushConfig"} {
		operations[name] = c.advertisedCapability(name, push)
	}
	operations["GetExtendedAgentCard"] = c.advertisedCapability("GetExtendedAgentCard", extended)
	return middleware.ProviderCapabilityReport{
		ProtocolKind: middleware.ProtocolKindA2A,
		Operations:   operations,
		Content: map[string]middleware.CapabilityDescriptor{
			"text": a2aCapability("text", true, "stable", "A2A Part.Text"),
			"raw":  a2aCapability("raw", true, "stable", "A2A Part.Raw"),
			"url":  a2aCapability("url", true, "stable", "A2A Part.URL"),
			"data": a2aCapability("data", true, "stable", "A2A Part.Data"),
		},
		Transports: map[string]middleware.CapabilityDescriptor{
			"JSONRPC":   a2aCapability("JSONRPC", true, "stable", "A2A JSON-RPC binding"),
			"HTTP+JSON": a2aCapability("HTTP+JSON", true, "stable", "A2A HTTP+JSON binding"),
			"GRPC":      a2aCapability("GRPC", false, "stable", "Matrix has no governed gRPC ingress listener"),
		},
	}
}

func (c *a2aConversationClient) advertisedCapability(name string, supported bool) middleware.CapabilityDescriptor {
	descriptor := a2aCapability(name, supported, "stable", "A2A v1.0.1 agent card")
	if c.advertised == nil {
		descriptor.Status = "unknown"
		descriptor.Source = "A2A endpoint configured without an agent card"
	}
	return descriptor
}

var _ middleware.ConversationCapabilityReporter = (*a2aConversationClient)(nil)
