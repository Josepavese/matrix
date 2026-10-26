// Package main implements the Matrix CLI.
package main

import (
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agent runtime overrides stored in SSOT",
	Long: `Manage effective agent configuration through the SSOT override layer.
Seed definitions stay in configs/agents*.json, while runtime mutations live in the vault.`,
}

func init() {
	rootCmd.AddCommand(agentCmd)
	// Consumer/publisher surfaces for the shared artifact store: what software is
	// installed, whether it verifies, how to rebuild it, and how to reclaim a
	// per-home copy once a verified generation holds.
	agentCmd.AddCommand(agentInventoryCmd, agentVerifyCmd, agentRestoreCmd, agentPruneCmd, agentStoreCmd)
}
