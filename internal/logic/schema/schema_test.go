package schema

import (
	"encoding/json"
	"testing"
)

type mockStorage struct {
	data map[string][]byte
}

func (m *mockStorage) Get(key string) ([]byte, error)  { return m.data[key], nil }
func (m *mockStorage) Delete(key string) error         { delete(m.data, key); return nil }
func (m *mockStorage) List(_ string) ([]string, error) { return nil, nil }
func (m *mockStorage) Set(key string, val []byte) error {
	if m.data == nil {
		m.data = map[string][]byte{}
	}
	m.data[key] = val
	return nil
}

func TestEnsureCurrentInitializesEmptyStorage(t *testing.T) {
	store := &mockStorage{data: map[string][]byte{}}
	report, err := EnsureCurrent(store)
	if err != nil {
		t.Fatalf("EnsureCurrent: %v", err)
	}
	if report.Status != "initialized" {
		t.Fatalf("expected initialized status, got %+v", report)
	}
	loaded, err := LoadReport(store)
	if err != nil {
		t.Fatalf("LoadReport: %v", err)
	}
	if loaded.StoredVersion != CurrentVersion || loaded.Status != "current" {
		t.Fatalf("unexpected loaded report: %+v", loaded)
	}
}

func TestEnsureCurrentMigratesOlderVersion(t *testing.T) {
	store := &mockStorage{data: map[string][]byte{}}
	data, err := json.Marshal(CurrentVersion + 1)
	if err != nil {
		t.Fatalf("Marshal(version): %v", err)
	}
	store.data[VersionKey] = data
	report, err := EnsureCurrent(store)
	if err != nil {
		t.Fatalf("EnsureCurrent: %v", err)
	}
	if report.Status != "migrated" || report.StoredVersion != CurrentVersion {
		t.Fatalf("unexpected report after migration: %+v", report)
	}
}
