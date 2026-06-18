package bolt

import (
	"bytes"
	"encoding/base64"
	"path/filepath"
	"testing"
)

func setTestVaultKey(t *testing.T) {
	t.Helper()
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)))
}

func TestProvider_SetAndGet(t *testing.T) {
	setTestVaultKey(t)
	// Create a temporary database file
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_vault.db")

	provider, err := NewProvider(dbPath)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	key := "test.key"
	val := []byte("test_value")

	// Test Set
	if err := provider.Set(key, val); err != nil {
		t.Fatalf("Failed to set key: %v", err)
	}

	// Test Get
	got, err := provider.Get(key)
	if err != nil {
		t.Fatalf("Failed to get key: %v", err)
	}

	if !bytes.Equal(got, val) {
		t.Errorf("Get() = %s, want %s", got, val)
	}
}

func TestProvider_GetNonExistent(t *testing.T) {
	setTestVaultKey(t)
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_vault2.db")

	provider, err := NewProvider(dbPath)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	got, err := provider.Get("does_not_exist")
	if err != nil {
		t.Fatalf("Unexpected error for non-existent key: %v", err)
	}
	if got != nil {
		t.Errorf("Expected nil for non-existent key, got %v", got)
	}
}

func TestProvider_SetRequiresVaultKey(t *testing.T) {
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	t.Setenv("MATRIX_HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "test_vault_no_key.db")
	provider, err := NewProvider(dbPath)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	if err := provider.Set("test.key", []byte("value")); err == nil {
		t.Fatalf("expected set without vault key to fail")
	}
}

func TestProvider_ConcurrentAccess(t *testing.T) {
	setTestVaultKey(t)
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_vault3.db")

	provider, err := NewProvider(dbPath)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	key := "concurrent.key"
	val := []byte("safe")

	if err := provider.Set(key, val); err != nil {
		t.Fatalf("Failed to set key: %v", err)
	}

	// Simple concurrency test
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			got, err := provider.Get(key)
			if err != nil {
				t.Errorf("Concurrent Get failed: %v", err)
			}
			if !bytes.Equal(got, val) {
				t.Errorf("Concurrent Get value mismatch")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
