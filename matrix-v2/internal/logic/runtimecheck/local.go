package runtimecheck

import (
	"context"

	"github.com/jose/matrix-v2/internal/middleware"
)

type LocalInput struct {
	VaultPath   string
	JSONRPCAddr string
	ACPHTTPAddr string
	Net         middleware.Network
	FS          middleware.FS
	// BuildInput is the fully wired input for BuildReport.
	// The cmd layer is responsible for creating Storage, Registry, Process, etc.
	BuildInput  *BuildInput
}

func BuildLocalReport(input LocalInput) (map[string]any, error) {
	ctx := context.Background()
	if CanDial(input.Net, input.ACPHTTPAddr) {
		if report, err := FetchRuntimeReport(ctx, input.Net, "http://"+input.ACPHTTPAddr+"/_matrix/runtime"); err == nil {
			return report, ValidateRuntimeReport(report)
		}
	}

	warnings := []string{}
	report := map[string]any{
		"vault_path":          input.VaultPath,
		"vault_exists":        false,
		"jsonrpc_daemon_addr": input.JSONRPCAddr,
		"jsonrpc_daemon_up":   CanDial(input.Net, input.JSONRPCAddr),
		"acp_http_addr":       input.ACPHTTPAddr,
		"acp_http_up":         CanDial(input.Net, input.ACPHTTPAddr),
		"telegram_enabled":    false,
		"telegram_configured": false,
		"telegram_source":     "",
		"agents":              []any{},
		"warnings":            warnings,
	}

	if _, err := input.FS.Stat(input.VaultPath); err == nil {
		report["vault_exists"] = true
	}

	if input.BuildInput == nil {
		report["warnings"] = append(warnings, "runtime context unavailable: no build input")
		return report, ValidateRuntimeReport(report)
	}

	return BuildReport(*input.BuildInput)
}
