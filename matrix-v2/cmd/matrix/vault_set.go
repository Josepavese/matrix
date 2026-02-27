package main

import (
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	"github.com/spf13/cobra"
)

var vaultSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a value in the Vault",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		provider, err := bolt.NewProvider("matrix-vault.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer provider.Close()

		v := vault.NewVault(provider)
		err = v.SetString(args[0], args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to set value: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully set %s\n", args[0])
	},
}

func init() {
	vaultCmd.AddCommand(vaultSetCmd)
}
