package config

import (
	"errors"
	"testing"

	"github.com/jose/matrix-v2/internal/logic/vault"
)

// mockStorage is a simple in-memory storage for testing config.Manager
type mockStorage struct {
	data map[string][]byte
}

func (m *mockStorage) Get(key string) ([]byte, error) {
	return m.data[key], nil
}
func (m *mockStorage) Set(key string, val []byte) error {
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	m.data[key] = val
	return nil
}
func (m *mockStorage) Delete(key string) error {
	delete(m.data, key)
	return nil
}
func (m *mockStorage) List(prefix string) ([]string, error) {
	var keys []string
	for k := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func newTestManager() *Manager {
	store := &mockStorage{data: make(map[string][]byte)}
	v := vault.NewVault(store)
	return NewManager(v)
}

func TestConfigManager_SetAndGet(t *testing.T) {
	mgr := newTestManager()

	if err := mgr.Set("provider.openai.key", "sk-test123"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, err := mgr.Get("provider.openai.key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "sk-test123" {
		t.Errorf("Get() = %q, want %q", val, "sk-test123")
	}
}

func TestConfigManager_GetMissing(t *testing.T) {
	mgr := newTestManager()

	val, err := mgr.Get("does.not.exist")
	if err != nil {
		t.Fatalf("Get for missing key should not error: %v", err)
	}
	if val != "" {
		t.Errorf("Expected empty string for missing key, got %q", val)
	}
}

func TestConfigManager_Delete(t *testing.T) {
	mgr := newTestManager()

	if err := mgr.Set("todelete", "value"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := mgr.Delete("todelete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	val, err := mgr.Get("todelete")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "" {
		t.Errorf("Expected empty after delete, got %q", val)
	}
}

func TestConfigManager_List(t *testing.T) {
	mgr := newTestManager()

	if err := mgr.Set("provider.openai.key", "sk-1"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := mgr.Set("provider.default", "openai"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	keys, err := mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d: %v", len(keys), keys)
	}
	// Keys should not have the internal "config." prefix
	for _, k := range keys {
		if len(k) >= 7 && k[:7] == "config." {
			t.Errorf("Key %q should not have 'config.' prefix in output", k)
		}
	}
}

func TestConfigManager_EmptyKey(t *testing.T) {
	mgr := newTestManager()

	if err := mgr.Set("", "val"); err == nil {
		t.Error("Expected error for empty key in Set, got nil")
	}
	if _, err := mgr.Get(""); err == nil {
		t.Error("Expected error for empty key in Get, got nil")
	}
	if err := mgr.Delete(""); err == nil {
		t.Error("Expected error for empty key in Delete, got nil")
	}
	// Silence unused import
	_ = errors.New
}
