package vaultsec

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestEncryptDecryptBytes(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(key))

	plain := []byte(`"secret"`)
	encrypted, err := EncryptBytes(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !IsEncryptedValue(encrypted) {
		t.Fatalf("expected encrypted value")
	}

	decrypted, err := DecryptBytes(encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decrypted) != string(plain) {
		t.Fatalf("unexpected decrypted payload: %s", string(decrypted))
	}
}

func TestDecryptEncryptedBytesWithoutKeyFails(t *testing.T) {
	key := bytes.Repeat([]byte{9}, 32)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(key))
	encrypted, err := EncryptBytes([]byte(`"secret"`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	if _, err := DecryptBytes(encrypted); err == nil {
		t.Fatalf("expected decrypt failure without key")
	}
}
