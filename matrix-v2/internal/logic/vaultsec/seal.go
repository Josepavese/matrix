package vaultsec

import "github.com/jose/matrix-v2/internal/middleware"

// SealStorage rewrites all vault entries with encryption.
func SealStorage(store middleware.Storage) (int, error) {
	keys, err := store.List("")
	if err != nil {
		return 0, err
	}
	count := 0
	for _, key := range keys {
		value, err := store.Get(key)
		if err != nil {
			return count, err
		}
		if err := store.Set(key, value); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
