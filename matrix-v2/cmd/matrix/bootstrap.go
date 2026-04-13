package main

import (
	"github.com/jose/matrix-v2/internal/logic/bootstrap"
	"github.com/jose/matrix-v2/internal/logic/cmdutil"
	"github.com/jose/matrix-v2/internal/logic/config"
	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var bootstrapCmd = &cobra.Command{Use: "bootstrap", Short: "Inspect and guide first-run setup"}

var bootstrapDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Show bootstrap readiness and setup guidance",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cleanup, err := NewAgentContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		report, err := bootstrap.BuildReport(ctx.Store, config.NewManager(vault.NewVault(ctx.Store)), ctx.Registry, osfs.NewConfigProvider())
		if err != nil {
			exitf("Error: %v", err)
		}
		if err := cmdutil.PrintJSON(cmd, report); err != nil {
			exitf("Error: %v", err)
		}
	},
}

func init() {
	bootstrapCmd.AddCommand(bootstrapDoctorCmd)
	rootCmd.AddCommand(bootstrapCmd)
}
