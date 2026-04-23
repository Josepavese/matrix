package readiness

import (
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/runtimecheck"
)

// Input groups the reports needed to evaluate local Matrix readiness.
type Input struct {
	RuntimeReport   map[string]any
	LoggingReport   map[string]any
	StorageReport   map[string]any
	VaultReport     map[string]any
	ExpectRuntimeUp bool
}

// Evaluate combines runtime, logging, storage, and vault reports into one readiness result.
func Evaluate(input Input) map[string]any {
	runtimeUp := localRuntimeEndpointsUp(input.RuntimeReport)
	vaultLockedByRuntime := storageUnavailableBecauseRuntimeOwnsVault(input.StorageReport, runtimeUp)
	blockers := collectBlockers(input, vaultLockedByRuntime)
	warnings := collectWarnings(input, vaultLockedByRuntime)
	status := statusFor(blockers, warnings)
	return report(status, blockers, warnings, input)
}

func collectBlockers(input Input, vaultLockedByRuntime bool) []string {
	blockers := []string{}
	if exists, ok := input.RuntimeReport["vault_exists"].(bool); !ok || !exists {
		blockers = append(blockers, "vault file is missing")
	}
	if input.ExpectRuntimeUp {
		if up, ok := input.RuntimeReport["jsonrpc_daemon_up"].(bool); !ok || !up {
			blockers = append(blockers, "jsonrpc daemon is not reachable")
		}
		if up, ok := input.RuntimeReport["matrix_http_up"].(bool); !ok || !up {
			blockers = append(blockers, "matrix http ingress is not reachable")
		}
	}
	if schemaMap, ok := input.StorageReport["schema"].(map[string]any); ok {
		if status, _ := schemaMap["status"].(string); status != "current" && !vaultLockedByRuntime {
			blockers = append(blockers, "vault schema is not current")
		}
	}
	return blockers
}

func collectWarnings(input Input, vaultLockedByRuntime bool) []string {
	warnings := []string{}
	if encryptionMap, ok := input.VaultReport["encryption"].(map[string]any); ok {
		if configured, ok := encryptionMap["configured"].(bool); !ok || !configured {
			warnings = append(warnings, "vault encryption master key is not configured")
		}
		if plaintext, ok := encryptionMap["plaintext_keys"].(int); ok && plaintext > 0 {
			warnings = append(warnings, "vault still contains plaintext entries")
		}
		if plaintextF, ok := encryptionMap["plaintext_keys"].(float64); ok && plaintextF > 0 {
			warnings = append(warnings, "vault still contains plaintext entries")
		}
	}
	appendWarnings(&warnings, input.LoggingReport["warnings"])
	if vaultLockedByRuntime {
		warnings = filterWarnings(warnings, isRuntimeVaultLockWarning)
	}
	runtimecheck.AppendReadinessWarnings(&warnings, input.RuntimeReport["warnings"], input.ExpectRuntimeUp)
	appendWorkspaceWarnings(&warnings, input.StorageReport["workspaces"])
	return warnings
}

func statusFor(blockers, warnings []string) string {
	if len(blockers) > 0 {
		return "not_ready"
	}
	if len(warnings) > 0 {
		return "ready_with_warnings"
	}
	return "ready"
}

func report(status string, blockers, warnings []string, input Input) map[string]any {
	return map[string]any{
		"status":   status,
		"blockers": blockers,
		"warnings": warnings,
		"checks": map[string]any{
			"runtime": input.RuntimeReport,
			"logging": input.LoggingReport,
			"storage": input.StorageReport,
			"vault":   input.VaultReport,
		},
	}
}

func appendWorkspaceWarnings(warnings *[]string, raw any) {
	switch workspaces := raw.(type) {
	case []map[string]any:
		for _, ws := range workspaces {
			addPruneWarning(warnings, ws)
		}
	case []any:
		for _, value := range workspaces {
			if ws, ok := value.(map[string]any); ok {
				addPruneWarning(warnings, ws)
			}
		}
	}
}

func localRuntimeEndpointsUp(runtimeReport map[string]any) bool {
	jsonrpcUp, _ := runtimeReport["jsonrpc_daemon_up"].(bool)
	httpUp, _ := runtimeReport["matrix_http_up"].(bool)
	return jsonrpcUp && httpUp
}

func storageUnavailableBecauseRuntimeOwnsVault(storageReport map[string]any, runtimeUp bool) bool {
	if !runtimeUp {
		return false
	}
	schemaMap, ok := storageReport["schema"].(map[string]any)
	if !ok {
		return false
	}
	status, _ := schemaMap["status"].(string)
	errText, _ := schemaMap["error"].(string)
	return status == "unavailable" && isVaultLockText(errText)
}

func filterWarnings(warnings []string, drop func(string) bool) []string {
	filtered := warnings[:0]
	for _, warning := range warnings {
		if !drop(warning) {
			filtered = append(filtered, warning)
		}
	}
	return filtered
}

func isRuntimeVaultLockWarning(warning string) bool {
	return strings.Contains(warning, "ERR_VAULT_OPEN") && isVaultLockText(warning)
}

func isVaultLockText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "timeout") || strings.Contains(lower, "database is locked")
}

func addPruneWarning(warnings *[]string, ws map[string]any) {
	workspaceID, _ := ws["id"].(string)
	if prunable, ok := ws["timeline_prunable"].(bool); ok && prunable {
		*warnings = append(*warnings, fmt.Sprintf("workspace %s has timeline data above retention", workspaceID))
	}
	if prunable, ok := ws["memory_prunable"].(bool); ok && prunable {
		*warnings = append(*warnings, fmt.Sprintf("workspace %s has memory data above retention", workspaceID))
	}
	if prunable, ok := ws["snapshots_prunable"].(bool); ok && prunable {
		*warnings = append(*warnings, fmt.Sprintf("workspace %s has snapshot data above retention", workspaceID))
	}
}

func appendWarnings(warnings *[]string, raw any) {
	switch values := raw.(type) {
	case []any:
		for _, value := range values {
			if text, ok := value.(string); ok {
				*warnings = append(*warnings, text)
			}
		}
	case []string:
		*warnings = append(*warnings, values...)
	}
}
