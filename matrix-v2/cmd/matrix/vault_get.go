package main

import (
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	"github.com/spf13/cobra"
)

var vaultGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a value from the Vault",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		provider, err := bolt.NewProvider("matrix-vault.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer provider.Close()

		v := vault.NewVault(provider)
		val, err := v.GetString(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get value: %v\n", err)
			os.Exit(1)
		}
		if val == "" {
			fmt.Fprintln(os.Stderr, "Not found")
			os.Exit(1)
		} else {
			fmt.Println(val)
		}
	},
}

func init() {
	vaultCmd.AddCommand(vaultGetCmd)
}
