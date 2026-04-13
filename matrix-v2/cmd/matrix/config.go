package main

import (
	"fmt"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/cmdutil"
	"github.com/jose/matrix-v2/internal/logic/config"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage Matrix V2 SSOT configuration",
	Long: `Manage Matrix V2 configuration stored in the SSOT Vault.
	All values are stored with dot-notation keys (e.g. provider.openai.key).`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Use: matrix config [set|get|delete|list]")
	},
}

func openConfigManager() (*config.Manager, func(), error) {
	provider, err := bolt.NewProvider(DefaultVaultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("vault error: %w", err)
	}
	return cmdutil.OpenConfigManagerFromStorage(provider), func() { _ = provider.Close() }, nil
}

func openReadOnlyConfigManager() (*config.Manager, func(), error) {
	provider, err := bolt.NewReadOnlyProvider(DefaultVaultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("vault error: %w", err)
	}
	return cmdutil.OpenConfigManagerFromStorage(provider), func() { _ = provider.Close() }, nil
}

func ensureConfigKeyAllowed(key string) {
	if strings.HasPrefix(strings.TrimSpace(key), "channel.") {
		exitf("Channel configuration must be managed with `matrix channel ...`, not `matrix config ...`")
	}
}

func init() {
	rootCmd.AddCommand(configCmd)
}
