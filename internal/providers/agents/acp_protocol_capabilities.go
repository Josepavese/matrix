package agents

import (
	"strconv"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// acpCapabilityFunc builds one descriptor with the generation's stability and
// source labelling already applied.
type acpCapabilityFunc func(name string, supported bool, origin string) middleware.CapabilityDescriptor

// ProtocolCapabilities reports what this connection can actually do. The report
// follows the generation the connection negotiated, because the two generations
// name and contain different methods: version 2 renames the authentication
// methods, removes session/load and session/set_mode, and deletes the client
// file system and terminal surfaces. Reporting the version 1 names on a version
// 2 connection would describe a surface the peer does not have.
//
// Stability follows generation too. Version 1 is the stable protocol; version 2
// is still an upstream draft, so every operation and content shape on a version
// 2 connection is reported as draft even when its name is the same as in
// version 1. Negotiation makes these operations available; it is not a claim
// that the draft has stabilized.
func (c *acpConversationClient) ProtocolCapabilities() middleware.ProviderCapabilityReport {
	version := c.negotiatedProtocolVersion()
	v2 := version >= zedacp.ProtocolVersionV2
	capability := acpCapabilityForGeneration(v2)
	report := middleware.ProviderCapabilityReport{
		ProtocolKind: middleware.ProtocolKindACP,
		Operations:   c.protocolOperations(v2, capability),
		Content:      c.protocolContent(capability),
		Transports: map[string]middleware.CapabilityDescriptor{
			c.endpoint.Transport: acpCapability(c.endpoint.Transport, true, "stable", "configured ACP transport"),
		},
	}
	if version > 0 {
		report.ProtocolVersion = strconv.Itoa(version)
	}
	return report
}

func acpCapabilityForGeneration(v2 bool) acpCapabilityFunc {
	stability, source := "stable", func(v1 string) string { return v1 }
	if v2 {
		stability, source = "draft", acpDraftSource
	}
	return func(name string, supported bool, origin string) middleware.CapabilityDescriptor {
		return acpCapability(name, supported, stability, source(origin))
	}
}

func acpDraftSource(v1 string) string {
	source := strings.ReplaceAll(v1, "ACP v1", "ACP v2 draft")
	if !strings.Contains(source, "ACP v2") {
		source = "ACP v2 draft: " + source
	}
	return source
}

// acpGenerationSupported applies the v1-only gate to a capability whose name
// survived into version 2 but whose surface did not.
func acpGenerationSupported(v2, supported bool) bool {
	return !v2 && supported
}

func (c *acpConversationClient) protocolOperations(v2 bool, capability acpCapabilityFunc) map[string]middleware.CapabilityDescriptor {
	login, logout := "authenticate", "logout"
	if v2 {
		login, logout = "auth/login", "auth/logout"
	}
	operations := map[string]middleware.CapabilityDescriptor{
		"initialize":                 capability("initialize", true, "ACP v1"),
		login:                        capability(login, len(c.AuthenticationMethods()) > 0, "ACP authMethods"),
		logout:                       capability(logout, c.featureCapabilities.logout, "ACP auth.logout"),
		"session/new":                capability("session/new", true, "ACP v1 baseline"),
		"session/load":               capability("session/load", acpGenerationSupported(v2, c.sessionCapabilities.Load), "ACP v1 loadSession"),
		"session/resume":             capability("session/resume", c.sessionCapabilities.Resume, "ACP v1 session.resume"),
		"session/list":               capability("session/list", c.sessionCapabilities.List, "ACP v1 session.list"),
		"session/delete":             capability("session/delete", c.sessionCapabilities.Delete, "ACP v1 session.delete"),
		"session/close":              capability("session/close", c.sessionCapabilities.Close, "ACP v1 session.close"),
		"session/prompt":             capability("session/prompt", true, "ACP v1 baseline"),
		"session/cancel":             capability("session/cancel", true, "ACP v1 baseline"),
		"session/set_mode":           capability("session/set_mode", !v2, "ACP v1 session modes"),
		"session/set_config_option":  capability("session/set_config_option", true, "ACP v1 config options"),
		"session/update":             capability("session/update", true, "ACP v1 baseline"),
		"session/request_permission": capability("session/request_permission", true, "ACP v1 client callback"),
		"fs/read_text_file":          capability("fs/read_text_file", acpGenerationSupported(v2, c.featureCapabilities.fsRead), "ACP v1 client capability"),
		"fs/write_text_file":         capability("fs/write_text_file", acpGenerationSupported(v2, c.featureCapabilities.fsWrite), "ACP v1 client capability"),
		"terminal/create":            capability("terminal/create", acpGenerationSupported(v2, c.featureCapabilities.terminal), "ACP v1 client capability"),
		"terminal/output":            capability("terminal/output", acpGenerationSupported(v2, c.featureCapabilities.terminal), "ACP v1 client capability"),
		"terminal/release":           capability("terminal/release", acpGenerationSupported(v2, c.featureCapabilities.terminal), "ACP v1 client capability"),
		"terminal/wait_for_exit":     capability("terminal/wait_for_exit", acpGenerationSupported(v2, c.featureCapabilities.terminal), "ACP v1 client capability"),
		"terminal/kill":              capability("terminal/kill", acpGenerationSupported(v2, c.featureCapabilities.terminal), "ACP v1 client capability"),
		"$/cancel_request":           capability("$/cancel_request", true, "ACP v1 JSON-RPC cancellation"),
		// Elicitation stabilized upstream on 2026-07-24. Availability follows
		// the neutral frontend port: advertised in initialize only when the
		// port is wired with a usable mode, otherwise inbound requests get an
		// explicit decline. The report must use the same predicate as the wire
		// advertisement, or it would promise a surface the adapter refuses.
		"elicitation/create": capability("elicitation/create", acpElicitationSupported(c.handler), "ACP v1 elicitation; advertised iff an elicitation frontend with a usable mode is wired"),
	}
	operations["session/fork"] = acpForkCapability(c.sessionCapabilities.Fork)
	return operations
}

func acpElicitationSupported(handler *defaultRequestHandler) bool {
	return handler != nil && elicitationAdvertisement(handler.elicitation) != nil
}

func (c *acpConversationClient) protocolContent(capability acpCapabilityFunc) map[string]middleware.CapabilityDescriptor {
	return map[string]middleware.CapabilityDescriptor{
		"text":          capability("text", true, "ACP v1 baseline"),
		"resource_link": capability("resource_link", true, "ACP v1 baseline"),
		"image":         capability("image", c.featureCapabilities.promptImage, "ACP v1 promptCapabilities.image"),
		"audio":         capability("audio", c.featureCapabilities.promptAudio, "ACP v1 promptCapabilities.audio"),
		"resource":      capability("resource", c.featureCapabilities.promptEmbeddedContext, "ACP v1 promptCapabilities.embeddedContext"),
	}
}

var _ middleware.ConversationCapabilityReporter = (*acpConversationClient)(nil)
