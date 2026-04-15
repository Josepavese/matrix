// Package cmdutil provides command-line utility functions for output formatting and config access.
package cmdutil

import (
	"github.com/jose/matrix-v2/internal/logic/config"
	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/middleware"
)

// OpenConfigManagerFromStorage creates a config manager from an already-opened storage provider.
// The caller is responsible for closing the storage provider.
func OpenConfigManagerFromStorage(store middleware.Storage) *config.Manager {
	return config.NewManager(vault.NewVault(store))
}
