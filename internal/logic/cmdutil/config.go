// Package cmdutil provides command-line utility functions for output formatting and config access.
package cmdutil

import (
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/logic/vault"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/runtimevault"
)

// OpenConfigManagerFromStorage creates a config manager from an already-opened storage provider.
// The caller is responsible for closing the storage provider.
func OpenConfigManagerFromStorage(store middleware.Storage) *config.Manager {
	return config.NewManager(vault.NewVault(store))
}

// OpenReadOnlyConfigManager uses the runtime broker when the daemon owns bbolt.
func OpenReadOnlyConfigManager(vaultPath string) (*config.Manager, func(), error) {
	provider, err := runtimevault.OpenReadOnly(vaultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("vault error: %w", err)
	}
	return OpenConfigManagerFromStorage(provider), func() { _ = provider.Close() }, nil
}
