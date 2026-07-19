package agents

import "github.com/Josepavese/matrix/internal/middleware"

func (c *acpConversationClient) ProtocolCapabilities() middleware.ProviderCapabilityReport {
	operations := map[string]middleware.CapabilityDescriptor{
		"initialize":                 acpCapability("initialize", true, "stable", "ACP v1"),
		"authenticate":               acpCapability("authenticate", len(c.AuthenticationMethods()) > 0, "stable", "ACP v1 authMethods"),
		"logout":                     acpCapability("logout", c.featureCapabilities.logout, "stable", "ACP v1 auth.logout"),
		"session/new":                acpCapability("session/new", true, "stable", "ACP v1 baseline"),
		"session/load":               acpCapability("session/load", c.sessionCapabilities.Load, "stable", "ACP v1 loadSession"),
		"session/resume":             acpCapability("session/resume", c.sessionCapabilities.Resume, "stable", "ACP v1 session.resume"),
		"session/list":               acpCapability("session/list", c.sessionCapabilities.List, "stable", "ACP v1 session.list"),
		"session/delete":             acpCapability("session/delete", c.sessionCapabilities.Delete, "stable", "ACP v1 session.delete"),
		"session/close":              acpCapability("session/close", c.sessionCapabilities.Close, "stable", "ACP v1 session.close"),
		"session/prompt":             acpCapability("session/prompt", true, "stable", "ACP v1 baseline"),
		"session/cancel":             acpCapability("session/cancel", true, "stable", "ACP v1 baseline"),
		"session/set_mode":           acpCapability("session/set_mode", true, "stable", "ACP v1 session modes"),
		"session/set_config_option":  acpCapability("session/set_config_option", true, "stable", "ACP v1 config options"),
		"session/update":             acpCapability("session/update", true, "stable", "ACP v1 baseline"),
		"session/request_permission": acpCapability("session/request_permission", true, "stable", "ACP v1 client callback"),
		"fs/read_text_file":          acpCapability("fs/read_text_file", c.featureCapabilities.fsRead, "stable", "ACP v1 client capability"),
		"fs/write_text_file":         acpCapability("fs/write_text_file", c.featureCapabilities.fsWrite, "stable", "ACP v1 client capability"),
		"terminal/create":            acpCapability("terminal/create", c.featureCapabilities.terminal, "stable", "ACP v1 client capability"),
		"terminal/output":            acpCapability("terminal/output", c.featureCapabilities.terminal, "stable", "ACP v1 client capability"),
		"terminal/release":           acpCapability("terminal/release", c.featureCapabilities.terminal, "stable", "ACP v1 client capability"),
		"terminal/wait_for_exit":     acpCapability("terminal/wait_for_exit", c.featureCapabilities.terminal, "stable", "ACP v1 client capability"),
		"terminal/kill":              acpCapability("terminal/kill", c.featureCapabilities.terminal, "stable", "ACP v1 client capability"),
		"$/cancel_request":           acpCapability("$/cancel_request", true, "stable", "ACP v1 JSON-RPC cancellation"),
	}
	operations["session/fork"] = acpForkCapability(c.sessionCapabilities.Fork)
	return middleware.ProviderCapabilityReport{
		ProtocolKind: middleware.ProtocolKindACP,
		Operations:   operations,
		Content: map[string]middleware.CapabilityDescriptor{
			"text":          acpCapability("text", true, "stable", "ACP v1 baseline"),
			"resource_link": acpCapability("resource_link", true, "stable", "ACP v1 baseline"),
			"image":         acpCapability("image", c.featureCapabilities.promptImage, "stable", "ACP v1 promptCapabilities.image"),
			"audio":         acpCapability("audio", c.featureCapabilities.promptAudio, "stable", "ACP v1 promptCapabilities.audio"),
			"resource":      acpCapability("resource", c.featureCapabilities.promptEmbeddedContext, "stable", "ACP v1 promptCapabilities.embeddedContext"),
		},
		Transports: map[string]middleware.CapabilityDescriptor{
			c.endpoint.Transport: acpCapability(c.endpoint.Transport, true, "stable", "configured ACP transport"),
		},
	}
}

var _ middleware.ConversationCapabilityReporter = (*acpConversationClient)(nil)
