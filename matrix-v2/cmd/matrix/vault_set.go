package main

import (
	"io"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/channelcfg"
	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	"github.com/spf13/cobra"
)

var vaultSetStdin bool

var vaultSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a value in the Vault",
	Args: func(cmd *cobra.Command, args []string) error {
		if vaultSetStdin {
			return cobra.ExactArgs(1)(cmd, args)
		}
		return cobra.ExactArgs(2)(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		provider, err := bolt.NewProvider(DefaultVaultPath)
		if err != nil {
			exitf("Vault error: %v", err)
		}
		defer func() { _ = provider.Close() }()

		v := vault.NewVault(provider)
		value := ""
		if vaultSetStdin {
			data, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				exitf("Failed to read stdin: %v", err)
			}
			value = strings.TrimRight(string(data), "\r\n")
		} else {
			value = args[1]
		}
		err = v.SetString(args[0], value)
		if err != nil {
			exitf("Failed to set value: %v", err)
		}
		if channelcfg.IsSecretKey(args[0]) {
			cmd.Printf("Successfully set %s = %s\n", args[0], channelcfg.RedactSecret(value))
			return
		}
		cmd.Printf("Successfully set %s\n", args[0])
	},
}

func init() {
	vaultSetCmd.Flags().BoolVar(&vaultSetStdin, "stdin", false, "read the value from stdin instead of argv")
	vaultCmd.AddCommand(vaultSetCmd)
}
