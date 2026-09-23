// Package runtimecheck provides runtime diagnostics and health checks.
package runtimecheck

import (
	"fmt"

	"github.com/Josepavese/matrix/internal/middleware"
)

// CanDial checks if an address is reachable.
func CanDial(net middleware.Network, address string) bool {
	return net.CanDial(address)
}

// AppendRuntimeWarnings appends default runtime warnings to the slice.
func AppendRuntimeWarnings(report map[string]any, warnings *[]string) {
	if !ReportBool(report, "vault_exists") {
		*warnings = append(*warnings, "vault database not found")
	}
	if !ReportBool(report, "jsonrpc_daemon_up") {
		*warnings = append(*warnings, "jsonrpc daemon is not reachable on 127.0.0.1:9090")
	}
	if !ReportBool(report, "matrix_http_up") {
		*warnings = append(*warnings, "matrix http ingress is not reachable on 127.0.0.1:9091")
	}
	if !ReportBool(report, "a2a_http_up") {
		*warnings = append(*warnings, "a2a http server is not reachable on 127.0.0.1:9091")
	}
	// The shipped default accepts unauthenticated requests on loopback. Nothing warned
	// about it, so an operator who later fronted the port with a reverse proxy had no
	// signal that the proxy had become the only thing between the internet and an agent
	// that installs software and writes configuration. A non-loopback bind without a key
	// is already refused at startup, so this speaks only about the accepted case.
	if addr, ok := report["matrix_http_addr"].(string); ok && !ReportBool(report, "matrix_http_authenticated") {
		if warning := UnauthenticatedLoopbackWarning(addr, "", "matrix_http_addr", "matrix_api_key"); warning != "" {
			*warnings = append(*warnings, warning)
		}
	}
}

// ValidateRuntimeReport checks that a runtime report has expected boolean fields.
func ValidateRuntimeReport(report map[string]any) error {
	for _, key := range []string{"vault_exists", "jsonrpc_daemon_up", "matrix_http_up", "a2a_http_up"} {
		if _, ok := report[key].(bool); !ok {
			return fmt.Errorf("invalid runtime doctor report: %s is not a bool", key)
		}
	}
	return nil
}

// ReportBool extracts a boolean from a report map.
func ReportBool(report map[string]any, key string) bool {
	value, ok := report[key].(bool)
	return ok && value
}
