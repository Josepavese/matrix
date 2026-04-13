// Package bolt implements a BoltDB-based storage provider for the Matrix vault.
package bolt

import (
	"fmt"
	"sync"
	"time"

	"github.com/jose/matrix-v2/internal/logic/vaultsec"
	"github.com/jose/matrix-v2/internal/middleware"
	bbolt "go.etcd.io/bbolt"
)

// Provider implements middleware.Storage using bbolt
type Provider struct {
	db   *bbolt.DB
	path string
	mu   sync.RWMutex
}

var defaultBucket = []byte("matrix_vault")

// NewProvider initializes and returns a new bbolt storage provider
func NewProvider(path string) (*Provider, error) {
	return openProvider(path, false)
}

// NewReadOnlyProvider opens the vault in read-only mode for concurrent inspection.
func NewReadOnlyProvider(path string) (*Provider, error) {
	return openProvider(path, true)
}

func openProvider(path string, readOnly bool) (*Provider, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{Timeout: 1 * time.Second, ReadOnly: readOnly})
	if err != nil {
		return nil, &middleware.Error{
			Code:    "ERR_VAULT_OPEN",
			Message: "Failed to open bbolt database",
			Op:      "bolt.NewProvider",
			Err:     err,
		}
	}

	if readOnly {
		return &Provider{
			db:   db,
			path: path,
		}, nil
	}

	// Ensure default bucket exists
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(defaultBucket)
		return err
	})

	if err != nil {
		_ = db.Close()
		return nil, &middleware.Error{
			Code:    "ERR_VAULT_INIT",
			Message: "Failed to initialize default bucket",
			Op:      "bolt.NewProvider",
			Err:     err,
		}
	}

	if err := vaultsec.ApplySecurePermissions(path); err != nil {
		_ = db.Close()
		return nil, &middleware.Error{
			Code:    "ERR_VAULT_PERMISSIONS",
			Message: "Failed to enforce secure vault permissions",
			Op:      "bolt.NewProvider",
			Err:     err,
		}
	}

	return &Provider{
		db:   db,
		path: path,
	}, nil
}

// Get retrieves a value by key
func (p *Provider) Get(key string) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var val []byte
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(defaultBucket)
		if b == nil {
			return &middleware.Error{
				Code:    "ERR_VAULT_CORRUPT",
				Message: "Default bucket not found",
				Op:      "bolt.Get",
			}
		}
		v := b.Get([]byte(key))
		if v != nil {
			// bbolt values are only valid within the transaction, map to new slice
			val = make([]byte, len(v))
			copy(val, v)
		}
		return nil
	})

	if err != nil {
		return nil, &middleware.Error{
			Code:    "ERR_VAULT_GET",
			Message: fmt.Sprintf("Failed to get key: %s", key),
			Op:      "bolt.Get",
			Err:     err,
		}
	}

	if val == nil {
		return nil, nil
	}

	val, err = vaultsec.DecryptBytes(val)
	if err != nil {
		return nil, &middleware.Error{
			Code:    "ERR_VAULT_DECRYPT",
			Message: fmt.Sprintf("Failed to decrypt key: %s", key),
			Op:      "bolt.Get",
			Err:     err,
		}
	}
	return val, nil
}

// Set stores a value by key
func (p *Provider) Set(key string, val []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	encrypted, err := vaultsec.EncryptBytes(val)
	if err != nil {
		return &middleware.Error{
			Code:    "ERR_VAULT_ENCRYPT",
			Message: fmt.Sprintf("Failed to encrypt key: %s", key),
			Op:      "bolt.Set",
			Err:     err,
		}
	}

	err = p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(defaultBucket)
		if b == nil {
			return &middleware.Error{
				Code:    "ERR_VAULT_CORRUPT",
				Message: "Default bucket not found",
				Op:      "bolt.Set",
			}
		}
		return b.Put([]byte(key), encrypted)
	})

	if err != nil {
		return &middleware.Error{
			Code:    "ERR_VAULT_SET",
			Message: fmt.Sprintf("Failed to set key: %s", key),
			Op:      "bolt.Set",
			Err:     err,
		}
	}

	return nil
}

// Delete removes a key from the vault
func (p *Provider) Delete(key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	err := p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(defaultBucket)
		if b == nil {
			return &middleware.Error{
				Code:    "ERR_VAULT_CORRUPT",
				Message: "Default bucket not found",
				Op:      "bolt.Delete",
			}
		}
		return b.Delete([]byte(key))
	})
	if err != nil {
		return &middleware.Error{
			Code:    "ERR_VAULT_DELETE",
			Message: fmt.Sprintf("Failed to delete key: %s", key),
			Op:      "bolt.Delete",
			Err:     err,
		}
	}
	return nil
}

// List returns all keys that start with the given prefix
func (p *Provider) List(prefix string) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var keys []string
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(defaultBucket)
		if b == nil {
			return &middleware.Error{
				Code:    "ERR_VAULT_CORRUPT",
				Message: "Default bucket not found",
				Op:      "bolt.List",
			}
		}
		c := b.Cursor()
		pfx := []byte(prefix)
		for k, _ := c.Seek(pfx); k != nil && len(k) >= len(pfx) && string(k[:len(pfx)]) == prefix; k, _ = c.Next() {
			keys = append(keys, string(k))
		}
		return nil
	})
	if err != nil {
		return nil, &middleware.Error{
			Code:    "ERR_VAULT_LIST",
			Message: fmt.Sprintf("Failed to list keys with prefix: %s", prefix),
			Op:      "bolt.List",
			Err:     err,
		}
	}
	return keys, nil
}

// Close gracefully closes the vault
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}
