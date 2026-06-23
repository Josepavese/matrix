package agentcfg

import (
	"strings"
	"testing"
)

type memStorage struct {
	data map[string][]byte
}

func newMemStorage() *memStorage {
	return &memStorage{data: make(map[string][]byte)}
}
func (m *memStorage) Get(key string) ([]byte, error)   { return m.data[key], nil }
func (m *memStorage) Set(key string, val []byte) error { m.data[key] = val; return nil }
func (m *memStorage) Delete(key string) error          { delete(m.data, key); return nil }
func (m *memStorage) List(prefix string) ([]string, error) {
	var keys []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func TestSaveAndLoad(t *testing.T) {
	s := newMemStorage()
	active := true
	err := Save(s, "test-agent", Override{Active: &active, Env: []string{"API_KEY=sk-test"}, AppendArgs: []string{"-c", "sandbox_mode=\"danger-full-access\""}})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	ov, err := Load(s, "test-agent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ov.Active == nil || !*ov.Active {
		t.Error("expected active=true")
	}
	if len(ov.Env) != 1 || ov.Env[0] != "API_KEY=sk-test" {
		t.Errorf("unexpected env: %v", ov.Env)
	}
	if len(ov.AppendArgs) != 2 || ov.AppendArgs[1] != "sandbox_mode=\"danger-full-access\"" {
		t.Errorf("unexpected append args: %v", ov.AppendArgs)
	}
}

func TestLoad_NotFound(t *testing.T) {
	s := newMemStorage()
	ov, err := Load(s, "nonexistent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ov.Active != nil {
		t.Error("expected nil Active for nonexistent agent")
	}
	if len(ov.Env) != 0 {
		t.Errorf("expected empty env, got: %v", ov.Env)
	}
	if len(ov.AppendArgs) != 0 {
		t.Errorf("expected empty append args, got: %v", ov.AppendArgs)
	}
}

func TestSave_ActiveToggle(t *testing.T) {
	s := newMemStorage()

	active := true
	if err := Save(s, "agent1", Override{Active: &active}); err != nil {
		t.Fatalf("save active: %v", err)
	}
	ov, _ := Load(s, "agent1")
	if ov.Active == nil || !*ov.Active {
		t.Error("expected active=true")
	}

	inactive := false
	if err := Save(s, "agent1", Override{Active: &inactive}); err != nil {
		t.Fatalf("save inactive: %v", err)
	}
	ov, _ = Load(s, "agent1")
	if ov.Active == nil || *ov.Active {
		t.Error("expected active=false")
	}
}

func TestUpsertEnv(t *testing.T) {
	envs := []string{"API_KEY=old", "MODEL=gpt-4"}

	// Update existing
	envs = UpsertEnv(envs, "API_KEY", "new")
	if len(envs) != 2 {
		t.Fatalf("expected 2 envs, got %d", len(envs))
	}
	if envs[0] != "API_KEY=new" {
		t.Errorf("expected API_KEY=new, got %s", envs[0])
	}

	// Add new
	envs = UpsertEnv(envs, "PROVIDER", "openrouter")
	if len(envs) != 3 {
		t.Fatalf("expected 3 envs, got %d", len(envs))
	}
	found := false
	for _, e := range envs {
		if e == "PROVIDER=openrouter" {
			found = true
		}
	}
	if !found {
		t.Error("PROVIDER not found in envs")
	}
}

func TestRemoveEnv(t *testing.T) {
	envs := []string{"API_KEY=sk-test", "MODEL=gpt-4", "PROVIDER=openrouter"}
	envs = RemoveEnv(envs, "MODEL")
	if len(envs) != 2 {
		t.Fatalf("expected 2 envs, got %d", len(envs))
	}
	for _, e := range envs {
		if strings.HasPrefix(e, "MODEL=") {
			t.Error("MODEL should have been removed")
		}
	}
}

func TestSaveMetaAndLoadMeta(t *testing.T) {
	s := newMemStorage()
	meta := Meta{
		ID:          "gemini",
		Name:        "Gemini",
		Version:     "1.0.0",
		Description: "Google AI agent",
		DistTypes:   []string{"npx"},
	}
	if err := SaveMeta(s, "gemini", meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}

	loaded, err := LoadMeta(s, "gemini")
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if loaded.Name != "Gemini" {
		t.Errorf("expected Name=Gemini, got %s", loaded.Name)
	}
	if loaded.Version != "1.0.0" {
		t.Errorf("expected Version=1.0.0, got %s", loaded.Version)
	}
	if len(loaded.DistTypes) != 1 || loaded.DistTypes[0] != "npx" {
		t.Errorf("unexpected DistTypes: %v", loaded.DistTypes)
	}
}

func TestListMetaIDs(t *testing.T) {
	s := newMemStorage()
	if err := SaveMeta(s, "alpha", Meta{ID: "alpha", Name: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveMeta(s, "beta", Meta{ID: "beta", Name: "Beta"}); err != nil {
		t.Fatal(err)
	}

	ids, err := ListMetaIDs(s)
	if err != nil {
		t.Fatalf("ListMetaIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	// Sorted
	if ids[0] != "alpha" || ids[1] != "beta" {
		t.Errorf("unexpected order: %v", ids)
	}
}

func TestDeleteEntry(t *testing.T) {
	s := newMemStorage()
	active := true
	if err := Save(s, "to-delete", Override{Active: &active}); err != nil {
		t.Fatal(err)
	}

	if err := DeleteEntry(s, "to-delete"); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}

	ov, _ := Load(s, "to-delete")
	if ov.Active != nil {
		t.Error("expected nil Active after delete")
	}
}

func TestDeleteMeta(t *testing.T) {
	s := newMemStorage()
	if err := SaveMeta(s, "to-delete", Meta{ID: "to-delete", Name: "DeleteMe"}); err != nil {
		t.Fatal(err)
	}

	if err := DeleteMeta(s, "to-delete"); err != nil {
		t.Fatalf("DeleteMeta: %v", err)
	}

	meta, _ := LoadMeta(s, "to-delete")
	if meta.Name != "" {
		t.Error("expected empty Meta after delete")
	}
}

func TestListAgentIDs(t *testing.T) {
	s := newMemStorage()
	active := true
	if err := Save(s, "zebra", Override{Active: &active}); err != nil {
		t.Fatal(err)
	}
	if err := Save(s, "alpha", Override{Active: &active}); err != nil {
		t.Fatal(err)
	}

	ids, err := ListAgentIDs(s)
	if err != nil {
		t.Fatalf("ListAgentIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if ids[0] != "alpha" || ids[1] != "zebra" {
		t.Errorf("unexpected order: %v", ids)
	}
}

func TestNilStorage(t *testing.T) {
	if _, err := Load(nil, "test"); err != nil {
		t.Errorf("Load with nil storage should not error: %v", err)
	}
	if err := Save(nil, "test", Override{}); err == nil {
		t.Error("Save with nil storage should error")
	}
	if _, err := LoadMeta(nil, "test"); err != nil {
		t.Errorf("LoadMeta with nil storage should not error: %v", err)
	}
}
