package agentdiscovery

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/Josepavese/matrix/internal/middleware"
)

type memStorage struct {
	values map[string][]byte
}

func (m *memStorage) Get(key string) ([]byte, error) { return m.values[key], nil }
func (m *memStorage) Set(key string, val []byte) error {
	m.values[key] = append([]byte(nil), val...)
	return nil
}
func (m *memStorage) Delete(key string) error { delete(m.values, key); return nil }
func (m *memStorage) List(prefix string) ([]string, error) {
	keys := make([]string, 0)
	for key := range m.values {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

type fakeNet struct {
	payloads map[string][]byte
}

func (f *fakeNet) Download(context.Context, string, string) error { return nil }
func (f *fakeNet) Listen(string, string) (middleware.ClosableListener, error) {
	return nil, nil
}
func (f *fakeNet) FetchJSON(_ context.Context, url string, target interface{}) error {
	return json.Unmarshal(f.payloads[url], target)
}
func (f *fakeNet) GetFreePort() (int, error)                           { return 0, nil }
func (f *fakeNet) Fetch(_ context.Context, url string) ([]byte, error) { return f.payloads[url], nil }
func (f *fakeNet) PostJSON(context.Context, string, interface{}) ([]byte, int, error) {
	return nil, 0, nil
}
func (f *fakeNet) CanDial(string) bool { return false }

func TestResolveAgentCardURL(t *testing.T) {
	got, err := ResolveAgentCardURL("https://agents.example.com/a2a")
	if err != nil {
		t.Fatalf("ResolveAgentCardURL returned error: %v", err)
	}
	if want := "https://agents.example.com/.well-known/agent-card.json"; got != want {
		t.Fatalf("ResolveAgentCardURL = %q, want %q", got, want)
	}
}

func TestA2ACardProviderGet(t *testing.T) {
	net := &fakeNet{
		payloads: map[string][]byte{
			"https://agents.example.com/.well-known/agent-card.json": []byte(`{
				"name":"Remote Planner",
				"description":"Plans work",
				"version":"1.2.3",
				"supportedInterfaces":[{"url":"https://agents.example.com/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}],
				"capabilities":{"streaming":true},
				"defaultInputModes":["text/plain"],
				"defaultOutputModes":["text/plain"],
				"skills":[{"id":"plan","name":"Plan","description":"Plan tasks","tags":["planning","orchestration"]}]
			}`),
		},
	}

	provider, err := NewProvider(SourceA2ACard, Options{Net: net})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	record, err := provider.Get(context.Background(), "https://agents.example.com")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if record.Kind != middleware.ProtocolKindA2A {
		t.Fatalf("Kind = %q, want %q", record.Kind, middleware.ProtocolKindA2A)
	}
	if record.Address != "https://agents.example.com/a2a" {
		t.Fatalf("Address = %q", record.Address)
	}
	if record.Transport != "JSONRPC" {
		t.Fatalf("Transport = %q", record.Transport)
	}
}

func TestLocalProviderSearch(t *testing.T) {
	store := &memStorage{values: make(map[string][]byte)}
	if err := agentcfg.SaveEntry(store, "planner", agentcfg.Entry{
		Config: agentcfg.Config{
			Kind:      "a2a",
			Transport: "JSONRPC",
			Address:   "https://planner.example.com/a2a",
		},
	}); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}
	if err := agentcfg.SaveMeta(store, "planner", agentcfg.Meta{
		ID:          "planner",
		Name:        "Planner",
		Description: "Plans multi-step work",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}

	registry, err := agentmgr.NewRegistry(nil, store)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	provider, err := NewProvider(SourceLocal, Options{Registry: registry, Storage: store})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	records, err := provider.Search(context.Background(), "plan")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].Kind != middleware.ProtocolKindA2A {
		t.Fatalf("Kind = %q", records[0].Kind)
	}
}

func TestA2ACatalogProviderSearch(t *testing.T) {
	net := &fakeNet{
		payloads: map[string][]byte{
			"https://catalog.example.com/a2a.json": []byte(`{
				"agents":[
					{"id":"planner","name":"Planner","description":"Plans work","transport":"JSONRPC","address":"https://planner.example.com/a2a","card_url":"https://planner.example.com/.well-known/agent-card.json","tags":["planning"]}
				]
			}`),
		},
	}
	provider, err := NewProvider(SourceA2ACatalog, Options{
		Net:        net,
		CatalogURL: "https://catalog.example.com/a2a.json",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	records, err := provider.Search(context.Background(), "plan")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].CardURL == "" {
		t.Fatalf("CardURL should not be empty")
	}
}
