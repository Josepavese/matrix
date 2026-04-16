package main

import (
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/cmdutil"
	"github.com/jose/matrix-v2/internal/logic/runtimecheck"
	"github.com/jose/matrix-v2/internal/logic/vaultsec"
	"github.com/jose/matrix-v2/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var readinessExpectRuntimeUp bool
var readinessStrict bool

var readinessCmd = &cobra.Command{
	Use:   "readiness",
	Short: "Evaluate whether Matrix meets the current local production-readiness baseline",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		runtimeReport, err := buildRuntimeDoctorReport()
		if err != nil {
			exitf("Runtime doctor failed: %v", err)
		}
		loggingReport, err := buildLogsDoctorReport()
		if err != nil {
			exitf("Logging doctor failed: %v", err)
		}
		storageReport, err := buildStorageDoctorReport()
		if err != nil {
			exitf("Storage doctor failed: %v", err)
		}
		vaultReport, err := vaultsec.BuildReport(osfs.NewFSProvider(), DefaultVaultPath)
		if err != nil {
			exitf("Vault doctor failed: %v", err)
		}

		report := evaluateReadiness(runtimeReport, loggingReport, storageReport, vaultReport, readinessExpectRuntimeUp)
		if err := cmdutil.PrintJSON(cmd, report); err != nil {
			exitf("failed to print readiness report: %v", err)
		}
		if report["status"] == "not_ready" || (readinessStrict && report["status"] != "ready") {
			os.Exit(2)
		}
	},
}

func init() {
	readinessCmd.Flags().BoolVar(&readinessExpectRuntimeUp, "expect-runtime-up", false, "treat an inactive local runtime as a readiness blocker")
	readinessCmd.Flags().BoolVar(&readinessStrict, "strict", false, "return non-zero unless readiness status is exactly ready")
	rootCmd.AddCommand(readinessCmd)
}

func evaluateReadiness(runtimeReport, loggingReport, storageReport, vaultReport map[string]any, expectRuntimeUp bool) map[string]any {
	blockers := []string{}
	warnings := []string{}

	if exists, ok := runtimeReport["vault_exists"].(bool); !ok || !exists {
		blockers = append(blockers, "vault file is missing")
	}
	if expectRuntimeUp {
		if up, ok := runtimeReport["jsonrpc_daemon_up"].(bool); !ok || !up {
			blockers = append(blockers, "jsonrpc daemon is not reachable")
		}
		if up, ok := runtimeReport["acp_http_up"].(bool); !ok || !up {
			blockers = append(blockers, "matrix http ingress is not reachable")
		}
	}
	if schemaMap, ok := storageReport["schema"].(map[string]any); ok {
		if status, _ := schemaMap["status"].(string); status != "current" {
			blockers = append(blockers, "vault schema is not current")
		}
	}
	if encryptionMap, ok := vaultReport["encryption"].(map[string]any); ok {
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
	appendWarnings(&warnings, loggingReport["warnings"])
	runtimecheck.AppendReadinessWarnings(&warnings, runtimeReport["warnings"], expectRuntimeUp)
	if workspaces, ok := storageReport["workspaces"].([]map[string]any); ok {
		for _, ws := range workspaces {
			addPruneWarning(&warnings, ws)
		}
	}
	if workspaces, ok := storageReport["workspaces"].([]any); ok {
		for _, raw := range workspaces {
			if ws, ok := raw.(map[string]any); ok {
				addPruneWarning(&warnings, ws)
			}
		}
	}

	status := "ready"
	if len(blockers) > 0 {
		status = "not_ready"
	} else if len(warnings) > 0 {
		status = "ready_with_warnings"
	}

	return map[string]any{
		"status":   status,
		"blockers": blockers,
		"warnings": warnings,
		"checks": map[string]any{
			"runtime": runtimeReport,
			"logging": loggingReport,
			"storage": storageReport,
			"vault":   vaultReport,
		},
	}
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
