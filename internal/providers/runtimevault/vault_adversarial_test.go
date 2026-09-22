package runtimevault

import (
	"encoding/base64"
	"path/filepath"
	"testing"
)

// isolate points MATRIX_HOME at a temporary home so broker discovery cannot find
// the operator's running runtime. Without it these tests would silently talk to a
// real vault, which is both an isolation bug and a way to damage real data.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("MATRIX_HOME", t.TempDir())
}

// TestOpenRejectsAnUnusablePath keeps a misconfigured vault path from producing a
// storage handle that fails at the first read, long after startup.
func TestOpenRejectsAnUnusablePath(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	for name, path := range map[string]string{
		"missing directory": filepath.Join(dir, "absent", "vault.db"),
		"directory itself":  dir,
	} {
		t.Run(name, func(t *testing.T) {
			storage, err := Open(path)
			if err == nil {
				_ = storage.Close()
				t.Fatalf("opening %q must fail", path)
			}
		})
	}
}

// TestOpenReadOnlyRequiresAnExistingVault mirrors the bolt rule: a read-only
// handle must never create a vault, or a typo would look like success.
func TestOpenReadOnlyRequiresAnExistingVault(t *testing.T) {
	isolate(t)
	missing := filepath.Join(t.TempDir(), "absent.db")
	storage, err := OpenReadOnly(missing)
	if err == nil {
		_ = storage.Close()
		t.Fatal("a read-only open of a missing vault must fail")
	}
}

// TestOpenWithoutABrokerRefusesToStoreInPlaintext is a security contract: with
// no runtime running and no master key configured, the local vault must refuse
// the write rather than silently persisting cleartext at rest.
func TestOpenWithoutABrokerRefusesToStoreInPlaintext(t *testing.T) {
	isolate(t)
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	path := filepath.Join(t.TempDir(), "vault.db")
	storage, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = storage.Close() }()
	if err := storage.Set("k", []byte("v")); err == nil {
		t.Fatal("storing without a master key must be refused, not written in the clear")
	}
}

// TestOpenWithoutABrokerSealsValuesAtRest is the positive side of the same
// contract: with a key, the value round trips and the raw store shows no
// plaintext key.
func TestOpenWithoutABrokerSealsValuesAtRest(t *testing.T) {
	isolate(t)
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(key))
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")

	path := filepath.Join(t.TempDir(), "vault.db")
	storage, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = storage.Close() }()

	if err := storage.Set("k", []byte("v")); err != nil {
		t.Fatalf("set: %v", err)
	}
	value, err := storage.Get("k")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(value) != "v" {
		t.Fatalf("round trip returned %q", value)
	}
	encrypted, plaintext, err := storage.InspectRawEncryption()
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if plaintext != 0 {
		t.Fatalf("the vault stored %d plaintext keys", plaintext)
	}
	if encrypted == 0 {
		t.Fatal("the vault stored no encrypted key")
	}
}
