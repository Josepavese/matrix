package agents

import (
	"context"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestClientCacheKeyIncludesLaunchArgs(t *testing.T) {
	base := clientCacheKey("codex", "/tmp/ws")
	xhigh := clientCacheKey("codex", "/tmp/ws", "-c", "model_reasoning_effort=\"xhigh\"")
	high := clientCacheKey("codex", "/tmp/ws", "-c", "model_reasoning_effort=\"high\"")
	if base == xhigh || xhigh == high {
		t.Fatalf("launch args must partition client cache keys: base=%q xhigh=%q high=%q", base, xhigh, high)
	}
}

func TestReapAgentClientMatchesLaunchArgVariantsByWorkspace(t *testing.T) {
	router := NewRouter(nil)
	client := &closableClient{}
	key := clientCacheKey("codex", "/tmp/ws", "-c", "model_reasoning_effort=\"xhigh\"")
	router.clients[key] = client

	reaped, err := router.ReapAgentClient(context.Background(), "codex", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentClient: %v", err)
	}
	if !reaped || !client.closed {
		t.Fatalf("expected launch-arg variant to be closed, reaped=%v closed=%v", reaped, client.closed)
	}
	if _, ok := router.clients[key]; ok {
		t.Fatalf("expected launch-arg variant to be evicted")
	}
}

func TestReconcileAgentClientsRetainsLaunchArgVariantByWorkspace(t *testing.T) {
	router := NewRouter(nil)
	client := &trackedClosableClient{remoteSessionIDs: []string{"remote-1"}}
	key := clientCacheKey("codex", "/tmp/ws", "-c", "model_reasoning_effort=\"xhigh\"")
	router.clients[key] = client

	result, err := router.ReconcileAgentClients(context.Background(), []middleware.AgentClientRef{{
		AgentID:         "codex",
		WorkspacePath:   "/tmp/ws",
		RemoteSessionID: "remote-1",
	}})
	if err != nil {
		t.Fatalf("ReconcileAgentClients: %v", err)
	}
	if len(result.Retained) != 1 || result.Retained[0].RemoteSessionID != "remote-1" {
		t.Fatalf("expected launch-arg variant to be retained, got %+v", result)
	}
	if client.closed {
		t.Fatalf("retained launch-arg variant must not be closed")
	}
}
