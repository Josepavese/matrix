package vault

import (
	"errors"
	"testing"
)

// mockStorage is a simple in-memory implementation of middleware.Storage for testing
type mockStorage struct {
	data map[string][]byte
}

func (m *mockStorage) Get(key string) ([]byte, error) {
	if m.data == nil {
		return nil, nil
	}
	if string(key) == "error_trigger" {
		return nil, errors.New("simulated error")
	}
	return m.data[key], nil
}

func (m *mockStorage) Set(key string, val []byte) error {
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	if string(key) == "fail_set" {
		return errors.New("simulated set error")
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

func TestVault_SetAndGetString(t *testing.T) {
	mock := &mockStorage{data: make(map[string][]byte)}
	v := NewVault(mock)

	key := "test.string.key"
	expectedVal := "my_secret_string"

	// Test SetString
	err := v.SetString(key, expectedVal)
	if err != nil {
		t.Fatalf("SetString failed: %v", err)
	}

	// Verify it was serialized correctly in mock storage
	rawVal := string(mock.data[key])
	expectedRaw := `"my_secret_string"` // JSON encoding wraps string in quotes
	if rawVal != expectedRaw {
		t.Errorf("Storage data = %s, want %s", rawVal, expectedRaw)
	}

	// Test GetString
	got, err := v.GetString(key)
	if err != nil {
		t.Fatalf("GetString failed: %v", err)
	}

	if got != expectedVal {
		t.Errorf("GetString() = %s, want %s", got, expectedVal)
	}
}

func TestVault_GetNonExistentString(t *testing.T) {
	mock := &mockStorage{data: make(map[string][]byte)}
	v := NewVault(mock)

	got, err := v.GetString("not_here")
	if err != nil {
		t.Fatalf("Unexpected error for missing key: %v", err)
	}

	if got != "" {
		t.Errorf("Expected empty string, got %s", got)
	}
}

func TestVault_GetInvalidJSON(t *testing.T) {
	mock := &mockStorage{
		data: map[string][]byte{
			"bad_json": []byte(`{invalid`),
		},
	}
	v := NewVault(mock)

	_, err := v.GetString("bad_json")
	if err == nil {
		t.Fatal("Expected error when parsing invalid JSON, got nil")
	}
}

func TestVault_StorageErrors(t *testing.T) {
	mock := &mockStorage{data: make(map[string][]byte)}
	v := NewVault(mock)

	// Test underlying storage error on Get
	_, err := v.GetString("error_trigger")
	if err == nil {
		t.Fatal("Expected storage error on GetString, got nil")
	}

	// Test underlying storage error on Set
	err = v.SetString("fail_set", "value")
	if err == nil {
		t.Fatal("Expected storage error on SetString, got nil")
	}
}
