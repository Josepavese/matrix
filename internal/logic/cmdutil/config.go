// Package cmdutil provides command-line utility functions for output formatting and config access.
package cmdutil

import (
	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/logic/vault"
	"github.com/Josepavese/matrix/internal/middleware"
)

// OpenConfigManagerFromStorage creates a config manager from an already-opened storage provider.
// The caller is responsible for closing the storage provider.
func OpenConfigManagerFromStorage(store middleware.Storage) *config.Manager {
	return config.NewManager(vault.NewVault(store))
}
