package vaultsec

import (
	"fmt"
	"strings"
)

// NewValueConverter snapshots the cipher once for a complete atomic migration.
// The retired unbound format is never accepted by the normal value reader.
func NewValueConverter() (func(string, []byte) ([]byte, error), error) {
	gcm, err := valueCipher()
	if err != nil {
		return nil, err
	}
	return func(storageKey string, raw []byte) ([]byte, error) {
		if IsEncryptedValue(raw) {
			if _, err := openValue(gcm, raw, encryptedPrefix, []byte(storageKey)); err != nil {
				return nil, err
			}
			return raw, nil
		}
		plain := raw
		if strings.HasPrefix(string(raw), "ENCV1:") {
			var err error
			plain, err = openValue(gcm, raw, "ENCV1:", nil)
			if err != nil {
				return nil, err
			}
		} else if strings.HasPrefix(string(raw), "ENCV") {
			return nil, fmt.Errorf("unsupported vault value format for key %s", storageKey)
		}
		return sealValue(gcm, storageKey, plain)
	}, nil
}
