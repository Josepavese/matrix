package main

import (
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/config"
	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage Matrix V2 SSOT configuration",
	Long: `Manage Matrix V2 configuration stored in the SSOT Vault.
All values are stored with dot-notation keys (e.g. provider.openai.key).`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Use: matrix config [set|get|delete|list]")
	},
}

// openConfigManager is a shared helper that opens the Vault and returns a config.Manager.
// exitOnError is true for CLI commands that should terminate on failure.
func openConfigManager() (*config.Manager, func(), error) {
	provider, err := bolt.NewProvider("matrix-vault.db")
	if err != nil {
		return nil, nil, fmt.Errorf("vault error: %w", err)
	}
	v := vault.NewVault(provider)
	mgr := config.NewManager(v)
	cleanup := func() { provider.Close() }
	return mgr, cleanup, nil
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func init() {
	rootCmd.AddCommand(configCmd)
}
