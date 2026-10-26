package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/spf13/cobra"
)

var (
	agentRestoreInventory  string
	agentRestoreAll        bool
	agentRestoreStore      string
	agentRestoreAllowDrift bool
	agentRestoreReplace    bool
	agentRestoreDryRun     bool
	agentRestoreJSON       bool
)

var agentRestoreCmd = &cobra.Command{
	Use:   "restore (--inventory <file> | --all) [--store <root>]",
	Short: "Reinstall the regenerable agent software an inventory describes",
	Long: `Reinstall regenerable agent software from a governed source: a verified store
generation for store-backed agents, the governed registry otherwise.

Restoring never adopts a different registry version unless --allow-version-drift is
passed, and it never installs from an arbitrary URL. --dry-run reports what would
happen without downloading or writing anything.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		inventoryPath := strings.TrimSpace(agentRestoreInventory)
		if inventoryPath == "" && !agentRestoreAll {
			exitf("Error: pass --inventory <file> or --all")
		}
		if inventoryPath != "" && agentRestoreAll {
			exitf("Error: --inventory and --all are mutually exclusive")
		}
		ctx, cleanup, err := NewInstallerContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		inventory, err := readRestoreInventory(ctx, inventoryPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		if agentRestoreJSON {
			// Install progress would corrupt the machine-readable report.
			ctx.Installer.SetProgressWriter(nil)
		}
		results, err := ctx.Installer.Restore(context.Background(), inventory, agentmgr.RestoreOptions{
			StoreRoot:         strings.TrimSpace(agentRestoreStore),
			AllowVersionDrift: agentRestoreAllowDrift,
			Replace:           agentRestoreReplace,
			DryRun:            agentRestoreDryRun,
		})
		if err != nil {
			exitf("Error: restore: %v", err)
		}
		if agentRestoreJSON {
			emitJSON(cmd, map[string]any{"results": results})
		} else {
			for _, line := range restoreLines(results) {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
		}
		if restoreAnyError(results) {
			os.Exit(1)
		}
	},
}

// readRestoreInventory builds the inventory to restore from --all or from the
// validated JSON document named by --inventory.
func readRestoreInventory(ctx *InstallerContext, inventoryPath string) (agentmgr.Inventory, error) {
	if inventoryPath == "" {
		return ctx.Installer.Inventory()
	}
	data, err := os.ReadFile(inventoryPath)
	if err != nil {
		return agentmgr.Inventory{}, fmt.Errorf("read inventory %s: %w", inventoryPath, err)
	}
	return parseInventoryDocument(data)
}

// parseInventoryDocument validates an inventory document. Unknown fields are
// allowed so a newer producer can add information, but the schema version must be
// one this build understands: a restore reads the document to reinstall software.
func parseInventoryDocument(data []byte) (agentmgr.Inventory, error) {
	var inventory agentmgr.Inventory
	if err := json.Unmarshal(data, &inventory); err != nil {
		return agentmgr.Inventory{}, fmt.Errorf("parse inventory: %w", err)
	}
	if inventory.SchemaVersion != agentmgr.InventorySchemaVersion {
		return agentmgr.Inventory{}, fmt.Errorf("unsupported inventory schema_version %d: this build reads schema_version %d", inventory.SchemaVersion, agentmgr.InventorySchemaVersion)
	}
	return inventory, nil
}

func restoreLines(results []agentmgr.RestoreResult) []string {
	lines := make([]string, 0, len(results))
	for _, result := range results {
		line := fmt.Sprintf("%s\t%s", result.ID, result.Action)
		if result.Detail != "" {
			line += "\t" + result.Detail
		}
		if result.Error != "" {
			line += "\terror: " + result.Error
		}
		lines = append(lines, line)
	}
	return lines
}

func restoreAnyError(results []agentmgr.RestoreResult) bool {
	for _, result := range results {
		if result.Error != "" {
			return true
		}
	}
	return false
}

func init() {
	agentRestoreCmd.Flags().StringVar(&agentRestoreInventory, "inventory", "", "Inventory JSON document to restore from")
	agentRestoreCmd.Flags().BoolVar(&agentRestoreAll, "all", false, "Restore from the inventory of this PAL home")
	agentRestoreCmd.Flags().StringVar(&agentRestoreStore, "store", "", "Shared artifact store to install store-backed agents from")
	agentRestoreCmd.Flags().BoolVar(&agentRestoreAllowDrift, "allow-version-drift", false, "Accept a registry version different from the one the inventory recorded")
	agentRestoreCmd.Flags().BoolVar(&agentRestoreReplace, "replace", false, "Reinstall agents that are already registered")
	agentRestoreCmd.Flags().BoolVar(&agentRestoreDryRun, "dry-run", false, "Report actions without installing anything")
	agentRestoreCmd.Flags().BoolVar(&agentRestoreJSON, "json", false, "Emit the restore results as JSON")
}
