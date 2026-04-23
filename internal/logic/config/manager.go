// Package config provides SSOT configuration management.
package config

import (
	"strings"

	"github.com/Josepavese/matrix/internal/logic/vault"
	"github.com/Josepavese/matrix/internal/middleware"
)

const keyPrefix = "config."

// Manager handles SSOT configuration operations.
// It stores config entries in the Vault using the "config." key prefix,
// keeping them namespaced away from secret vault entries.
type Manager struct {
	vault *vault.Vault
}

// NewManager creates a new config manager backed by the given vault.
func NewManager(v *vault.Vault) *Manager {
	return &Manager{vault: v}
}

// Set stores a configuration value identified by dot-notation key.
// Example: Set("provider.openai.key", "sk-abc123")
func (m *Manager) Set(key, value string) error {
	if key == "" {
		return &middleware.Error{
			Code:    "ERR_CONFIG_KEY_EMPTY",
			Message: "Configuration key must not be empty",
			Op:      "config.Set",
		}
	}
	return m.vault.SetString(keyPrefix+key, value)
}

// Get retrieves a configuration value by dot-notation key.
// Returns empty string without error if the key does not exist.
func (m *Manager) Get(key string) (string, error) {
	if key == "" {
		return "", &middleware.Error{
			Code:    "ERR_CONFIG_KEY_EMPTY",
			Message: "Configuration key must not be empty",
			Op:      "config.Get",
		}
	}
	return m.vault.GetString(keyPrefix + key)
}

// GetWithDefault retrieves a configuration value, returning the fallback if missing.
func (m *Manager) GetWithDefault(key, fallback string) string {
	val, err := m.Get(key)
	if err != nil || val == "" {
		return fallback
	}
	return val
}

// Delete removes a configuration entry by dot-notation key.
func (m *Manager) Delete(key string) error {
	if key == "" {
		return &middleware.Error{
			Code:    "ERR_CONFIG_KEY_EMPTY",
			Message: "Configuration key must not be empty",
			Op:      "config.Delete",
		}
	}
	return m.vault.Delete(keyPrefix + key)
}

// List returns all configuration keys (without the internal prefix).
func (m *Manager) List() ([]string, error) {
	keys, err := m.vault.ListWithPrefix(keyPrefix)
	if err != nil {
		return nil, err
	}
	// Strip the internal "config." prefix for user-facing output
	result := make([]string, 0, len(keys))
	for _, k := range keys {
		result = append(result, strings.TrimPrefix(k, keyPrefix))
	}
	return result, nil
}
