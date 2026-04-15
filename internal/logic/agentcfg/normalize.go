package agentcfg

import (
	"strings"

	"github.com/jose/matrix-v2/internal/middleware"
)

// NormalizeEndpoint maps legacy agent configuration fields to the protocol-neutral endpoint model.
func NormalizeEndpoint(cfg Config) middleware.ProtocolEndpoint {
	kind := strings.ToLower(strings.TrimSpace(cfg.Kind))
	transport := strings.TrimSpace(cfg.Transport)

	legacyProtocol := strings.ToLower(strings.TrimSpace(cfg.Protocol))
	switch {
	case kind == "":
		switch legacyProtocol {
		case "", "acp":
			kind = string(middleware.ProtocolKindACP)
			if transport == "" {
				transport = "stdio"
			}
		case "stdio", "ws", "unix", "http":
			kind = string(middleware.ProtocolKindACP)
			if transport == "" {
				transport = legacyProtocol
			}
		case "a2a":
			kind = string(middleware.ProtocolKindA2A)
		default:
			kind = string(middleware.ProtocolKindACP)
			if transport == "" {
				transport = legacyProtocol
			}
		}
	case transport == "":
		switch legacyProtocol {
		case "stdio", "ws", "unix", "http":
			transport = legacyProtocol
		case "acp":
			transport = "stdio"
		case "a2a":
			transport = "JSONRPC"
		}
	}

	if strings.EqualFold(kind, string(middleware.ProtocolKindA2A)) {
		if transport == "" {
			transport = "JSONRPC"
		}
	}

	return middleware.ProtocolEndpoint{
		Kind:            middleware.ProtocolKind(kind),
		Transport:       transport,
		Address:         cfg.Address,
		Command:         cfg.Command,
		Args:            append([]string{}, cfg.Args...),
		Env:             append([]string{}, cfg.Env...),
		ProtocolVersion: cfg.ProtocolVersion,
		CardURL:         cfg.CardURL,
	}
}
