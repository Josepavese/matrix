package vaultsec

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jose/matrix-v2/internal/middleware"
)

const encryptedPrefix = "ENCV1:"

type KeyStatus struct {
	Configured bool
	Source     string
	Algorithm  string
}

func ResolveMasterKey(fs middleware.FS) ([]byte, KeyStatus, error) {
	if filePath := strings.TrimSpace(os.Getenv("MATRIX_VAULT_MASTER_KEY_FILE")); filePath != "" {
		if fs == nil {
			return nil, KeyStatus{}, fmt.Errorf("MATRIX_VAULT_MASTER_KEY_FILE is set but filesystem provider is nil")
		}
		data, err := fs.ReadFile(filePath)
		if err != nil {
			return nil, KeyStatus{}, err
		}
		key, err := parseMasterKey(string(data))
		if err != nil {
			return nil, KeyStatus{}, err
		}
		return key, KeyStatus{Configured: true, Source: "env:MATRIX_VAULT_MASTER_KEY_FILE", Algorithm: "aes-256-gcm"}, nil
	}
	if raw := strings.TrimSpace(os.Getenv("MATRIX_VAULT_MASTER_KEY")); raw != "" {
		key, err := parseMasterKey(raw)
		if err != nil {
			return nil, KeyStatus{}, err
		}
		return key, KeyStatus{Configured: true, Source: "env:MATRIX_VAULT_MASTER_KEY", Algorithm: "aes-256-gcm"}, nil
	}
	return nil, KeyStatus{Configured: false}, nil
}

func EncryptBytes(plain []byte) ([]byte, error) {
	key, _, err := ResolveMasterKey(nil)
	if err != nil {
		return nil, err
	}
	if len(key) == 0 {
		return plain, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plain, nil)
	payload := append(append([]byte{}, nonce...), sealed...)
	encoded := base64.StdEncoding.EncodeToString(payload)
	return []byte(encryptedPrefix + encoded), nil
}

func DecryptBytes(raw []byte) ([]byte, error) {
	if !IsEncryptedValue(raw) {
		return raw, nil
	}

	key, _, err := ResolveMasterKey(nil)
	if err != nil {
		return nil, err
	}
	if len(key) == 0 {
		return nil, errors.New("vault master key is required to decrypt encrypted values")
	}

	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(string(raw), encryptedPrefix))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize() {
		return nil, errors.New("encrypted payload is truncated")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func IsEncryptedValue(raw []byte) bool {
	return strings.HasPrefix(string(raw), encryptedPrefix)
}

func parseMasterKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	return nil, fmt.Errorf("vault master key must be 32 bytes encoded as base64 or hex")
}
