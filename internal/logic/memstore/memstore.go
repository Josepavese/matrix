// Package memstore provides a small in-memory middleware.Storage.
package memstore

import (
	"strings"
	"sync"
)

type Storage struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func New() *Storage {
	return &Storage{data: map[string][]byte{}}
}

func (m *Storage) Get(key string) ([]byte, error) {
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

func (m *Storage) Set(key string, val []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]byte, len(val))
	copy(out, val)
	m.data[key] = out
	return nil
}

func (m *Storage) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *Storage) List(prefix string) ([]string, error) {
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
