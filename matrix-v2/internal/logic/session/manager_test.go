package session

import (
	"context"
	"testing"

	"github.com/jose/matrix-v2/internal/logic/onboarding"
	"github.com/jose/matrix-v2/internal/middleware"
)

// mockStorage is a simple in-memory storage for testing Session routing
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

// mockRouter records the last message received
type mockRouter struct {
	lastSession string
	lastMsg     string
}

func (m *mockRouter) Route(_ context.Context, req middleware.RouteRequest) (string, string, []middleware.ToolCall, error) {
	m.lastSession = req.LogicalSessionID
	m.lastMsg = req.Message
	return "Ok", req.AgentSessionID, nil, nil
}

func TestSessionManager_GetOrCreate(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, onboarding.NewWizard(onboarding.WizardDependencies{Storage: storage}), nil)

	// Create a new session for telegram channel
	sessID1, err := mgr.GetOrCreateSession("telegram_1", "codex")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	if sessID1 == "" {
		t.Fatal("Expected a non-empty session ID")
	}

	// Repeated calls on the same channel should yield the same SessionID
	sessID2, err := mgr.GetOrCreateSession("telegram_1", "codex")
	if err != nil {
		t.Fatalf("Failed to retrieve existing session: %v", err)
	}
	if sessID1 != sessID2 {
		t.Errorf("Expected session %s to match %s", sessID1, sessID2)
	}
}

func TestSessionManager_Route(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, onboarding.NewWizard(onboarding.WizardDependencies{Storage: storage}), nil)

	msg := "Hello"
	res, err := mgr.Route(context.Background(), "web_token_abc", "test-agent", msg, nil)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if res != "Ok" {
		t.Errorf("Expected completed status, got %s", res)
	}

	// Verify the router received the right payload
	if router.lastSession == "" {
		t.Error("AgentRouter did not receive a SessionID")
	}
	if router.lastMsg != "Hello" {
		t.Errorf("AgentRouter received unexpected input: %+v", router.lastMsg)
	}
}

func TestSessionManager_Attach(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, onboarding.NewWizard(onboarding.WizardDependencies{Storage: storage}), nil)

	// Channel A generates Session 1
	sess1, err := mgr.GetOrCreateSession("channelA", "assistant")
	if err != nil {
		t.Fatalf("Failed to get or create session for channelA: %v", err)
	}

	// Verify we can attach Channel B to Session 1
	err = mgr.AttachChannel("channelB", sess1)
	if err != nil {
		t.Fatalf("Failed to attach channel: %v", err)
	}

	// Channel B should now route to Session 1
	sessB, err := mgr.GetOrCreateSession("channelB", "assistant")
	if err != nil {
		t.Fatalf("Failed to get or create session for channelB: %v", err)
	}
	if sess1 != sessB {
		t.Errorf("Attach failed. Channel B points to %s, expected %s", sessB, sess1)
	}
}
