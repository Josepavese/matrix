package main

import (
	"github.com/jose/matrix-v2/internal/logic/cmdutil"
	"github.com/jose/matrix-v2/internal/logic/schema"
	"github.com/spf13/cobra"
)

var vaultMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Initialize or migrate the vault schema to the current version",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		ctx, closeFn, err := NewAppContext(DefaultVaultPath)
		if err != nil {
			exitf("failed to open vault: %v", err)
		}
		defer closeFn()

		report, err := schema.EnsureCurrent(ctx.Store)
		if err != nil {
			exitf("vault migration failed: %v", err)
		}
		if err := cmdutil.PrintJSON(cmd, report); err != nil {
			exitf("failed to print migration report: %v", err)
		}
	},
}

func init() {
	vaultCmd.AddCommand(vaultMigrateCmd)
}
