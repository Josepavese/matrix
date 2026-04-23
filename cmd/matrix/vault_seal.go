package main

import (
	"github.com/Josepavese/matrix/internal/logic/vaultsec"
	"github.com/Josepavese/matrix/internal/providers/bolt"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var vaultSealCmd = &cobra.Command{
	Use:   "seal",
	Short: "Rewrite all vault entries with encryption using the configured master key",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		_, status, err := vaultsec.ResolveMasterKey(osfs.NewFSProvider())
		if err != nil {
			exitf("Vault encryption key error: %v", err)
		}
		if !status.Configured {
			exitf("Vault encryption master key is not configured")
		}

		provider, err := bolt.NewProvider(DefaultVaultPath)
		if err != nil {
			exitf("Vault error: %v", err)
		}
		defer func() { _ = provider.Close() }()

		count, err := vaultsec.SealStorage(provider)
		if err != nil {
			exitf("Vault seal failed: %v", err)
		}
		cmd.Printf("sealed %d vault entries using %s\n", count, status.Algorithm)
	},
}

func init() {
	vaultCmd.AddCommand(vaultSealCmd)
}
