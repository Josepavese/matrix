package agentcfg

import (
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// NormalizeEndpoint maps stored agent configuration to the protocol-neutral endpoint model.
func NormalizeEndpoint(cfg Config) middleware.ProtocolEndpoint {
	kind := strings.ToLower(strings.TrimSpace(cfg.Kind))
	transport := strings.TrimSpace(cfg.Transport)

	if kind == "" {
		kind = string(middleware.ProtocolKindACP)
	}

	if strings.EqualFold(kind, string(middleware.ProtocolKindA2A)) {
		if transport == "" {
			transport = "JSONRPC"
		}
	} else if transport == "" {
		transport = "stdio"
	}

	return middleware.ProtocolEndpoint{
		Kind:            middleware.ProtocolKind(kind),
		Transport:       transport,
		Address:         cfg.Address,
		Command:         cfg.Command,
		Args:            append([]string{}, cfg.Args...),
		Env:             append([]string{}, cfg.Env...),
		Headers:         CloneHeaders(cfg.Headers),
		Tenant:          strings.TrimSpace(cfg.Tenant),
		EnvIsolation:    cfg.EnvIsolation,
		ProtocolVersion: cfg.ProtocolVersion,
		CardURL:         cfg.CardURL,
	}
}

// CloneHeaders copies governed endpoint headers without sharing mutable state.
func CloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		out[key] = value
	}
	return out
}
