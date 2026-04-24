package runtimecheck

import (
	"context"
	"fmt"

	"github.com/Josepavese/matrix/internal/middleware"
)

// LocalInput holds parameters for building a local runtime report.
type LocalInput struct {
	VaultPath      string
	JSONRPCAddr    string
	MatrixHTTPAddr string
	A2AHTTPAddr    string
	Net            middleware.Network
	FS             middleware.FS
	// BuildInput is the fully wired input for BuildReport.
	// The cmd layer is responsible for creating Storage, Registry, Process, etc.
	BuildInput *BuildInput
}

// BuildLocalReport builds a runtime report by probing local endpoints.
func BuildLocalReport(input LocalInput) (map[string]any, error) {
	ctx := context.Background()
	warnings := []string{}
	if CanDial(input.Net, input.MatrixHTTPAddr) {
		if report, err := FetchRuntimeReport(ctx, input.Net, "http://"+input.MatrixHTTPAddr+"/_matrix/runtime"); err == nil {
			if err := ValidateRuntimeReport(report); err == nil {
				return report, nil
			} else {
				warnings = append(warnings, fmt.Sprintf("runtime endpoint report invalid; using local probe fallback: %v", err))
			}
		} else {
			warnings = append(warnings, fmt.Sprintf("runtime endpoint report unavailable; using local probe fallback: %v", err))
		}
	}

	report := map[string]any{
		"vault_path":          input.VaultPath,
		"vault_exists":        false,
		"jsonrpc_daemon_addr": input.JSONRPCAddr,
		"jsonrpc_daemon_up":   CanDial(input.Net, input.JSONRPCAddr),
		"matrix_http_addr":    input.MatrixHTTPAddr,
		"matrix_http_up":      CanDial(input.Net, input.MatrixHTTPAddr),
		"a2a_http_addr":       input.A2AHTTPAddr,
		"a2a_http_up":         CanDial(input.Net, input.A2AHTTPAddr),
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

	fullReport, err := BuildReport(*input.BuildInput)
	if err != nil {
		return nil, err
	}
	appendReportWarnings(fullReport, warnings)
	return fullReport, ValidateRuntimeReport(fullReport)
}

func appendReportWarnings(report map[string]any, warnings []string) {
	if len(warnings) == 0 {
		return
	}
	existing, _ := report["warnings"].([]string)
	report["warnings"] = append(existing, warnings...)
}
