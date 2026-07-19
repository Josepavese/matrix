package agentmgr

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentidentity"
)

type registryMemStorage struct {
	data map[string][]byte
}

func newRegistryMemStorage() *registryMemStorage {
	return &registryMemStorage{data: map[string][]byte{}}
}

func (m *registryMemStorage) Get(key string) ([]byte, error)   { return m.data[key], nil }
func (m *registryMemStorage) Set(key string, val []byte) error { m.data[key] = val; return nil }
func (m *registryMemStorage) Delete(key string) error          { delete(m.data, key); return nil }
func (m *registryMemStorage) List(prefix string) ([]string, error) {
	keys := []string{}
	for key := range m.data {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func TestNewRegistryAppendsOverrideArgs(t *testing.T) {
	store := newRegistryMemStorage()
	if err := agentcfg.SaveEntry(store, "codex", agentcfg.Entry{
		Config: agentcfg.Config{
			Command:   "codex-acp",
			Kind:      "acp",
			Transport: "stdio",
			Args:      []string{"--base"},
		},
		Override: agentcfg.Override{
			AppendArgs: []string{"-c", "sandbox_mode=\"danger-full-access\"", "-c", "approval_policy=\"never\""},
		},
	}); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}

	registry, err := NewRegistry(nil, store)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	cfg, err := registry.Get("codex")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := []string{"--base", "-c", "sandbox_mode=\"danger-full-access\"", "-c", "approval_policy=\"never\""}
	if strings.Join(cfg.Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected args: got %#v want %#v", cfg.Args, want)
	}
}

func TestNewRegistryRejectsProviderIdentifierAsPersistedAgentID(t *testing.T) {
	store := newRegistryMemStorage()
	if err := agentcfg.SaveEntry(store, "codex-acp", agentcfg.Entry{
		Config: agentcfg.Config{Command: "node", Kind: "acp", Transport: "stdio"},
	}); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}

	_, err := NewRegistry(nil, store)
	if err == nil || !strings.Contains(err.Error(), `use "codex"`) {
		t.Fatalf("expected canonical agent ID error, got %v", err)
	}
}

func TestNewRegistryRejectsDeprecatedCodexProvider(t *testing.T) {
	store := newRegistryMemStorage()
	if err := agentcfg.SaveEntry(store, "codex", agentcfg.Entry{
		Config: agentcfg.Config{
			Command: "npx", Args: []string{"-y", agentidentity.DeprecatedCodexPackage + "@0.16.0"},
			Kind: "acp", Transport: "stdio",
		},
	}); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}

	_, err := NewRegistry(nil, store)
	if err == nil || !strings.Contains(err.Error(), "matrix install codex") {
		t.Fatalf("expected ZERO-LEGACY migration error, got %v", err)
	}
}

func TestRegistryGetRejectsProviderIdentifier(t *testing.T) {
	registry := &Registry{configs: map[string]AgentConfig{}}
	_, err := registry.Get("codex-acp")
	if err == nil || !strings.Contains(err.Error(), `use "codex"`) {
		t.Fatalf("expected canonical agent ID error, got %v", err)
	}
}
