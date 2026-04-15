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
}
