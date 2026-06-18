package vaultsec

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
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

func TestEncryptBytesWithoutKeyFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")

	if _, err := EncryptBytes([]byte(`"secret"`)); err == nil {
		t.Fatalf("expected encrypt failure without key")
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

func TestEnsureDefaultMasterKeyCreatesMatrixHomeKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")

	status, err := EnsureDefaultMasterKey(nil)
	if err != nil {
		t.Fatalf("ensure default key: %v", err)
	}
	if !status.Configured || status.Source != "matrix_home:configs/vault-master.key" {
		t.Fatalf("unexpected key status: %#v", status)
	}

	path := filepath.Join(home, "configs", "vault-master.key")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected key file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key file permissions = %o, want 600", info.Mode().Perm())
	}

	key, resolved, err := ResolveMasterKey(nil)
	if err != nil {
		t.Fatalf("resolve generated key: %v", err)
	}
	if !resolved.Configured || len(key) != 32 {
		t.Fatalf("unexpected resolved key status=%#v len=%d", resolved, len(key))
	}
}

func TestResolveMasterKeyRejectsBroadFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode assertion")
	}
	key := bytes.Repeat([]byte{4}, 32)
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")

	keyDir := filepath.Join(home, "configs")
	if err := os.MkdirAll(keyDir, 0o755); err != nil {
		t.Fatalf("mkdir key dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(keyDir, "vault-master.key"), []byte(base64.StdEncoding.EncodeToString(key)), 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}

	if _, _, err := ResolveMasterKey(nil); err == nil {
		t.Fatalf("expected broad key file permissions to fail")
	}
}
