// Package vault provides encrypted key-value vault operations.
package vault

import (
	"encoding/json"

	"github.com/Josepavese/matrix/internal/middleware"
)

// Vault handles SSOT parameters mapping onto a middleware.Storage provider.
type Vault struct {
	store middleware.Storage
}

// NewVault creates a new Vault manager logic layer.
func NewVault(store middleware.Storage) *Vault {
	return &Vault{store: store}
}

// GetString retrieves a string value from the vault.
func (v *Vault) GetString(key string) (string, error) {
	data, err := v.store.Get(key)
	if err != nil {
		return "", err
	}
	if data == nil {
		return "", nil // Or maybe return a specific Not Found error
	}
	var val string
	if err := json.Unmarshal(data, &val); err != nil {
		return "", &middleware.Error{
			Code:    "ERR_VAULT_PARSE",
			Message: "Failed to parse value as string",
			Op:      "vault.GetString",
			Err:     err,
		}
	}
	return val, nil
}

// SetString stores a string value in the vault.
func (v *Vault) SetString(key, val string) error {
	data, err := json.Marshal(val)
	if err != nil {
		return &middleware.Error{
			Code:    "ERR_VAULT_SERIALIZE",
			Message: "Failed to serialize string value",
			Op:      "vault.SetString",
			Err:     err,
		}
	}
	return v.store.Set(key, data)
}

// Delete removes a key from the vault.
func (v *Vault) Delete(key string) error {
	return v.store.Delete(key)
}

// ListWithPrefix returns all keys that start with the given prefix.
func (v *Vault) ListWithPrefix(prefix string) ([]string, error) {
	return v.store.List(prefix)
}
