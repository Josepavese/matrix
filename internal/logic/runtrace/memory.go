package runtrace

import (
	"strings"
	"sync"
)

// MemoryStorage is a small in-memory middleware.Storage implementation used by
// HTTP tests and by embedded servers that have not wired the vault yet.
type MemoryStorage struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{data: map[string][]byte{}}
}

func (m *MemoryStorage) Get(key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val := m.data[key]
	if val == nil {
		return nil, nil
	}
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

func (m *MemoryStorage) Set(key string, val []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]byte, len(val))
	copy(out, val)
	m.data[key] = out
	return nil
}

func (m *MemoryStorage) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *MemoryStorage) List(prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0)
	for key := range m.data {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}
