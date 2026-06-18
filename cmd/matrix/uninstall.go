package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall <agent_id>",
	Short: "Uninstall an AI agent and remove its local files and registration",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		agentID := args[0]

		// 1. Setup Dependencies
		ctx, cleanup, err := NewInstallerContext(DefaultVaultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer cleanup()

		// 2. Execute Uninstall
		if err := ctx.Installer.Uninstall(context.Background(), agentID); err != nil {
			fmt.Fprintf(os.Stderr, "Uninstallation failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully uninstalled agent: %s\n", agentID)
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
