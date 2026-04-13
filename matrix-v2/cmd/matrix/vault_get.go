package main

import (
	"github.com/jose/matrix-v2/internal/logic/channelcfg"
	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	"github.com/spf13/cobra"
)

var vaultGetReveal bool

var vaultGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a value from the Vault",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		provider, err := bolt.NewReadOnlyProvider(DefaultVaultPath)
		if err != nil {
			exitf("Vault error: %v", err)
		}
		defer provider.Close()

		v := vault.NewVault(provider)
		val, err := v.GetString(args[0])
		if err != nil {
			exitf("Failed to get value: %v", err)
		}
		if val == "" {
			exitf("Not found")
		}
		if !vaultGetReveal && channelcfg.IsSecretKey(args[0]) {
			cmd.Println(channelcfg.RedactSecret(val))
			return
		}
		cmd.Println(val)
	},
}

func init() {
	vaultGetCmd.Flags().BoolVar(&vaultGetReveal, "reveal", false, "print the raw value without redaction")
	vaultCmd.AddCommand(vaultGetCmd)
}
