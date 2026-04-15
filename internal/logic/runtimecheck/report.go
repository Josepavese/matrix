package runtimecheck

import (
	"github.com/jose/matrix-v2/internal/logic/agentmgr"
	"github.com/jose/matrix-v2/internal/logic/channelcfg"
	"github.com/jose/matrix-v2/internal/logic/config"
	"github.com/jose/matrix-v2/internal/middleware"
)

// BuildInput holds the full set of dependencies for building a runtime report.
type BuildInput struct {
	Store         middleware.Storage
	Registry      *agentmgr.Registry
	Process       middleware.Process
	ConfigManager *config.Manager
	ConfigReader  middleware.ConfigReader
	Net           middleware.Network
	JSONRPCAddr   string
	ACPHTTPAddr   string
	A2AHTTPAddr   string
}

// BuildReport generates a comprehensive runtime report.
func BuildReport(input BuildInput) (map[string]any, error) {
	tgCfg, source, err := channelcfg.LoadTelegramConfig(input.ConfigReader, input.ConfigManager)
	if err != nil {
		return nil, err
	}

	canDial := func(addr string) bool { return CanDial(input.Net, addr) }
	reports, warnings, err := agentmgr.BuildRuntimeReports(input.Store, input.Registry, input.Process, canDial)
	if err != nil {
		return nil, err
	}

	report := map[string]any{
		"vault_path":          "matrix-vault.db",
		"vault_exists":        true,
		"jsonrpc_daemon_addr": input.JSONRPCAddr,
		"jsonrpc_daemon_up":   CanDial(input.Net, input.JSONRPCAddr),
		"acp_http_addr":       input.ACPHTTPAddr,
		"acp_http_up":         CanDial(input.Net, input.ACPHTTPAddr),
		"a2a_http_addr":       input.A2AHTTPAddr,
		"a2a_http_up":         CanDial(input.Net, input.A2AHTTPAddr),
		"telegram_enabled":    tgCfg.Enabled,
		"telegram_configured": tgCfg.Token != "",
		"telegram_source":     source,
		"agents":              reports,
		"warnings":            warnings,
	}
	AppendRuntimeWarnings(report, &warnings)
	report["warnings"] = warnings
	return report, ValidateRuntimeReport(report)
}
