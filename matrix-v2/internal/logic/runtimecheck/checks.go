package runtimecheck

import (
	"fmt"

	"github.com/jose/matrix-v2/internal/middleware"
)

func CanDial(net middleware.Network, address string) bool {
	return net.CanDial(address)
}

func AppendRuntimeWarnings(report map[string]any, warnings *[]string) {
	if !ReportBool(report, "vault_exists") {
		*warnings = append(*warnings, "vault database not found")
	}
	if !ReportBool(report, "jsonrpc_daemon_up") {
		*warnings = append(*warnings, "jsonrpc daemon is not reachable on 127.0.0.1:9090")
	}
	if !ReportBool(report, "acp_http_up") {
		*warnings = append(*warnings, "acp http server is not reachable on 127.0.0.1:9091")
	}
}

func ValidateRuntimeReport(report map[string]any) error {
	for _, key := range []string{"vault_exists", "jsonrpc_daemon_up", "acp_http_up"} {
		if _, ok := report[key].(bool); !ok {
			return fmt.Errorf("invalid runtime doctor report: %s is not a bool", key)
		}
	}
	return nil
}

func ReportBool(report map[string]any, key string) bool {
	value, ok := report[key].(bool)
	return ok && value
}
