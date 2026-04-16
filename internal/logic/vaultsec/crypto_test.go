package vaultsec

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
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
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
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

func TestResolveMasterKeyUsesMatrixHomeDefaultFile(t *testing.T) {
	key := bytes.Repeat([]byte{3}, 32)
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")

	keyDir := filepath.Join(home, "configs")
	if err := os.MkdirAll(keyDir, 0o755); err != nil {
		t.Fatalf("mkdir key dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(keyDir, "vault-master.key"), []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	resolved, status, err := ResolveMasterKey(nil)
	if err != nil {
		t.Fatalf("resolve key: %v", err)
	}
	if !bytes.Equal(resolved, key) {
		t.Fatalf("unexpected key")
	}
	if !status.Configured {
		t.Fatalf("expected configured status")
	}
	if status.Source != "matrix_home:configs/vault-master.key" {
		t.Fatalf("unexpected source: %s", status.Source)
	}
}
