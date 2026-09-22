package bolt

import (
	"path/filepath"
	"testing"
)

// TestReadOnlyProviderRequiresAnExistingVault keeps a read-only open from
// silently creating an empty database: an operator who points the runtime at the
// wrong path would otherwise get a working, empty vault instead of an error.
func TestReadOnlyProviderRequiresAnExistingVault(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.db")
	provider, err := NewReadOnlyProvider(missing)
	if err == nil {
		_ = provider.Close()
		t.Fatal("a read-only open of a missing vault must fail")
	}
}

// TestNewProviderCreatesAUsableVault covers the write path: the file appears, the
// store answers, and closing it twice is safe.
func TestNewProviderCreatesAUsableVault(t *testing.T) {
	// The vault refuses to store without a key, and the operator's home must not
	// be what supplies one: this test failed on CI for exactly that reason.
	setTestVaultKey(t)
	path := filepath.Join(t.TempDir(), "vault.db")
	provider, err := NewProvider(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := provider.Set("k", []byte("v")); err != nil {
		t.Fatalf("set: %v", err)
	}
	value, err := provider.Get("k")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(value) != "v" {
		t.Fatalf("round trip returned %q", value)
	}
	if err := provider.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// A second open must see the persisted value, which is the whole point of a
	// durable store.
	reopened, err := NewReadOnlyProvider(path)
	if err != nil {
		t.Fatalf("reopen read-only: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	value, err = reopened.Get("k")
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if string(value) != "v" {
		t.Fatalf("reopened vault lost the value, got %q", value)
	}
}

// TestUnreadableVaultPathIsReported keeps a directory or an unwritable location
// from being reported as a healthy vault.
func TestUnreadableVaultPathIsReported(t *testing.T) {
	directory := t.TempDir()
	provider, err := NewProvider(directory)
	if err == nil {
		_ = provider.Close()
		t.Fatal("opening a directory as a vault must fail")
	}
	if _, err := NewProvider(filepath.Join(directory, "missing", "vault.db")); err == nil {
		t.Fatal("opening a vault in a missing directory must fail")
	}
}
