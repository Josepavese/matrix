package agentcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentdiscovery"
	"github.com/Josepavese/matrix/internal/middleware"
)

type testStorage struct {
	data map[string][]byte
}

func newTestStorage() *testStorage {
	return &testStorage{data: make(map[string][]byte)}
}

func (s *testStorage) Get(key string) ([]byte, error) { return s.data[key], nil }
func (s *testStorage) Set(key string, val []byte) error {
	s.data[key] = val
	return nil
}
func (s *testStorage) Delete(key string) error {
	delete(s.data, key)
	return nil
}
func (s *testStorage) List(prefix string) ([]string, error) {
	out := []string{}
	for key := range s.data {
		if len(prefix) == 0 || len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			out = append(out, key)
		}
	}
	return out, nil
}

type testNet struct {
	payloads map[string]interface{}
}

func (n *testNet) Listen(_, _ string) (middleware.ClosableListener, error) { return nil, nil }
func (n *testNet) Download(_ context.Context, _, _ string) error           { return nil }
func (n *testNet) GetFreePort() (int, error)                               { return 0, nil }
func (n *testNet) Fetch(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}
func (n *testNet) PostJSON(_ context.Context, _ string, _ interface{}) ([]byte, int, error) {
	return nil, 0, fmt.Errorf("not implemented")
}
func (n *testNet) CanDial(_ string) bool { return false }
func (n *testNet) FetchJSON(_ context.Context, url string, target interface{}) error {
	payload, ok := n.payloads[url]
	if !ok {
		return fmt.Errorf("missing payload for %s", url)
	}
	switch dst := target.(type) {
	case *map[string]interface{}:
		mapped, ok := payload.(map[string]interface{})
		if !ok {
			return fmt.Errorf("payload for %s is not an object", url)
		}
		*dst = mapped
		return nil
	default:
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, target)
	}
}

func TestServiceListMergesLocalAndA2ACatalog(t *testing.T) {
	store := newTestStorage()
	if err := agentcfg.SaveMeta(store, "codex", agentcfg.Meta{ID: "codex", Name: "Codex", DistTypes: []string{"npx"}}); err != nil {
		t.Fatalf("SaveMeta codex: %v", err)
	}
	if err := agentcfg.SaveEntry(store, "codex", agentcfg.Entry{Config: agentcfg.Config{Kind: "acp", Transport: "stdio", Command: "codex-acp"}}); err != nil {
		t.Fatalf("SaveEntry codex: %v", err)
	}

	net := &testNet{
		payloads: map[string]interface{}{
			"https://catalog.example.com/agents.json": map[string]interface{}{
				"entries": []map[string]interface{}{
					{
						"id":        "codex",
						"name":      "Codex Remote",
						"kind":      "a2a",
						"transport": "JSONRPC",
						"address":   "https://codex.example.com/a2a",
					},
					{
						"id":        "planner",
						"name":      "Planner",
						"kind":      "a2a",
						"transport": "JSONRPC",
						"address":   "https://planner.example.com/a2a",
					},
				},
			},
		},
	}

	service := NewService(Config{
		Storage:        store,
		Net:            net,
		Sources:        []agentdiscovery.Source{agentdiscovery.SourceLocal, agentdiscovery.SourceA2ACatalog},
		A2ACatalogURLs: []string{"https://catalog.example.com/agents.json"},
	})

	entries, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 merged entries, got %d", len(entries))
	}
	if entries[0].ID != "codex" || !entries[0].Installed || entries[0].Source != agentdiscovery.SourceLocal {
		t.Fatalf("expected local codex to win merge, got %+v", entries[0])
	}
	if entries[1].ID != "planner" || entries[1].Source != agentdiscovery.SourceA2ACatalog {
		t.Fatalf("expected planner from A2A catalog, got %+v", entries[1])
	}
}

func TestRegisterRemotePersistsA2AEndpoint(t *testing.T) {
	store := newTestStorage()
	err := RegisterRemote(store, Entry{
		ID:              "remote-planner",
		Name:            "Remote Planner",
		Description:     "A2A planner",
		Source:          agentdiscovery.SourceA2ACard,
		Kind:            middleware.ProtocolKindA2A,
		Transport:       "JSONRPC",
		Address:         "http://127.0.0.1:8088/a2a",
		CardURL:         "http://127.0.0.1:8088/.well-known/agent-card.json",
		ProtocolVersion: "1.0",
	})
	if err != nil {
		t.Fatalf("RegisterRemote: %v", err)
	}

	entry, err := agentcfg.LoadEntry(store, "remote-planner")
	if err != nil {
		t.Fatalf("LoadEntry: %v", err)
	}
	if entry.Config.Kind != "a2a" || entry.Config.Address != "http://127.0.0.1:8088/a2a" {
		t.Fatalf("unexpected entry config: %+v", entry.Config)
	}
	meta, err := agentcfg.LoadMeta(store, "remote-planner")
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.Name != "Remote Planner" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}
