package agents

import (
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// ProtocolCapabilities reports what this connection can actually do. The report
// follows the generation the connection negotiated, because the two generations
// name and contain different methods: version 2 renames the authentication
// methods, removes session/load and session/set_mode, and deletes the client
// file system and terminal surfaces, and reporting the version 1 names on a
// version 2 connection would describe a surface the peer does not have.
func (c *acpConversationClient) ProtocolCapabilities() middleware.ProviderCapabilityReport {
	v2 := c.negotiatedProtocolVersion() >= zedacp.ProtocolVersionV2
	login, logout := "authenticate", "logout"
	if v2 {
		login, logout = "auth/login", "auth/logout"
	}
	operations := map[string]middleware.CapabilityDescriptor{
		"initialize":                 acpCapability("initialize", true, "stable", "ACP v1"),
		login:                        acpCapability(login, len(c.AuthenticationMethods()) > 0, "stable", "ACP authMethods"),
		logout:                       acpCapability(logout, c.featureCapabilities.logout, "stable", "ACP auth.logout"),
		"session/new":                acpCapability("session/new", true, "stable", "ACP v1 baseline"),
		"session/load":               acpCapability("session/load", !v2 && c.sessionCapabilities.Load, "stable", "ACP v1 loadSession"),
		"session/resume":             acpCapability("session/resume", c.sessionCapabilities.Resume, "stable", "ACP v1 session.resume"),
		"session/list":               acpCapability("session/list", c.sessionCapabilities.List, "stable", "ACP v1 session.list"),
		"session/delete":             acpCapability("session/delete", c.sessionCapabilities.Delete, "stable", "ACP v1 session.delete"),
		"session/close":              acpCapability("session/close", c.sessionCapabilities.Close, "stable", "ACP v1 session.close"),
		"session/prompt":             acpCapability("session/prompt", true, "stable", "ACP v1 baseline"),
		"session/cancel":             acpCapability("session/cancel", true, "stable", "ACP v1 baseline"),
		"session/set_mode":           acpCapability("session/set_mode", !v2, "stable", "ACP v1 session modes"),
		"session/set_config_option":  acpCapability("session/set_config_option", true, "stable", "ACP v1 config options"),
		"session/update":             acpCapability("session/update", true, "stable", "ACP v1 baseline"),
		"session/request_permission": acpCapability("session/request_permission", true, "stable", "ACP v1 client callback"),
		"fs/read_text_file":          acpCapability("fs/read_text_file", !v2 && c.featureCapabilities.fsRead, "stable", "ACP v1 client capability"),
		"fs/write_text_file":         acpCapability("fs/write_text_file", !v2 && c.featureCapabilities.fsWrite, "stable", "ACP v1 client capability"),
		"terminal/create":            acpCapability("terminal/create", !v2 && c.featureCapabilities.terminal, "stable", "ACP v1 client capability"),
		"terminal/output":            acpCapability("terminal/output", !v2 && c.featureCapabilities.terminal, "stable", "ACP v1 client capability"),
		"terminal/release":           acpCapability("terminal/release", !v2 && c.featureCapabilities.terminal, "stable", "ACP v1 client capability"),
		"terminal/wait_for_exit":     acpCapability("terminal/wait_for_exit", !v2 && c.featureCapabilities.terminal, "stable", "ACP v1 client capability"),
		"terminal/kill":              acpCapability("terminal/kill", !v2 && c.featureCapabilities.terminal, "stable", "ACP v1 client capability"),
		"$/cancel_request":           acpCapability("$/cancel_request", true, "stable", "ACP v1 JSON-RPC cancellation"),
		// Elicitation stabilized upstream on 2026-07-24. Availability follows
		// the neutral frontend port: advertised in initialize only when the
		// port is wired with a usable mode, otherwise inbound requests get an
		// explicit decline. The report must use the same predicate as the wire
		// advertisement, or it would promise a surface the adapter refuses.
		"elicitation/create": acpCapability("elicitation/create", c.handler != nil && elicitationAdvertisement(c.handler.elicitation) != nil, "stable", "ACP v1 elicitation; advertised iff an elicitation frontend with a usable mode is wired"),
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
