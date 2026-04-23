package agents

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

type recoveryClient struct {
	newSessionCalls int
	promptCalls     int
}

func (c *recoveryClient) Alive() bool  { return true }
func (c *recoveryClient) Close() error { return nil }

func (c *recoveryClient) ExecuteTurn(_ context.Context, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	c.promptCalls++
	if c.promptCalls == 1 && turn.RemoteSessionID != "" {
		return middleware.ConversationResult{}, fmt.Errorf("ACP prompt failed: RPC error -32602: Invalid params: Session not found: stale-session")
	}
	c.newSessionCalls++
	return middleware.ConversationResult{
		Output:          "recovered",
		RemoteSessionID: fmt.Sprintf("session-%d", c.newSessionCalls),
	}, nil
}

func TestRouter_ExecutePrompt_RecoversFromMissingSession(t *testing.T) {
	router := NewRouter(nil)
	client := &recoveryClient{}

	output, sessionID, _, _, err := router.executePrompt(context.Background(), client, middleware.RouteRequest{
		LogicalSessionID: "logical-session",
		AgentSessionID:   "stale-session",
		Message:          "hello",
	})
	if err != nil {
		t.Fatalf("executePrompt failed: %v", err)
	}
	if output != "recovered" {
		t.Fatalf("expected recovered output, got %q", output)
	}
	if sessionID != "session-1" {
		t.Fatalf("expected new session id session-1, got %q", sessionID)
	}
	if client.newSessionCalls != 1 {
		t.Fatalf("expected 1 remote session assignment, got %d", client.newSessionCalls)
	}
	if client.promptCalls != 2 {
		t.Fatalf("expected 2 prompt calls, got %d", client.promptCalls)
	}
}

func TestRouter_ExecutePrompt_StrictSessionDoesNotRecoverFromMissingSession(t *testing.T) {
	router := NewRouter(nil)
	client := &recoveryClient{}

	_, sessionID, _, _, err := router.executePrompt(context.Background(), client, middleware.RouteRequest{
		LogicalSessionID: "logical-session",
		AgentSessionID:   "stale-session",
		Message:          "hello",
		StrictSession:    true,
	})
	if err == nil {
		t.Fatalf("expected missing session error")
	}
	if sessionID != "" {
		t.Fatalf("strict live routing must not create replacement session, got %q", sessionID)
	}
	if client.promptCalls != 1 || client.newSessionCalls != 0 {
		t.Fatalf("expected one failed prompt and no recovery, prompt=%d new=%d", client.promptCalls, client.newSessionCalls)
	}
}

type failingSessionClient struct{}

func (c failingSessionClient) Close() error { return nil }

func (c failingSessionClient) ExecuteTurn(context.Context, middleware.ConversationTurn) (middleware.ConversationResult, error) {
	return middleware.ConversationResult{RemoteSessionID: "remote-before-error"}, fmt.Errorf("prompt failed")
}

func TestRouter_ExecutePrompt_ReturnsRemoteSessionIDOnPromptError(t *testing.T) {
	router := NewRouter(nil)

	_, sessionID, _, _, err := router.executePrompt(context.Background(), failingSessionClient{}, middleware.RouteRequest{
		LogicalSessionID: "logical-session",
		Message:          "hello",
	})
	if err == nil {
		t.Fatalf("expected prompt error")
	}
	if sessionID != "remote-before-error" {
		t.Fatalf("expected remote session id to be preserved for cleanup, got %q", sessionID)
	}
}

type closableClient struct {
	closed bool
}

func (c *closableClient) ExecuteTurn(context.Context, middleware.ConversationTurn) (middleware.ConversationResult, error) {
	return middleware.ConversationResult{}, nil
}

func (c *closableClient) Close() error {
	c.closed = true
	return nil
}

type deadClosableClient struct {
	closableClient
}

func (c *deadClosableClient) Alive() bool { return false }

func TestRouter_ReapAgentClientClosesExactWorkspaceClient(t *testing.T) {
	router := NewRouter(nil)
	client := &closableClient{}
	key := clientCacheKey("opencode", "/tmp/ws")
	router.clients[key] = client

	reaped, err := router.ReapAgentClient(context.Background(), "opencode", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentClient: %v", err)
	}
	if !reaped || !client.closed {
		t.Fatalf("expected client to be closed, reaped=%v closed=%v", reaped, client.closed)
	}
	if _, ok := router.clients[key]; ok {
		t.Fatalf("expected client cache entry to be evicted")
	}
}

func TestRouter_ReconcileAgentClientsReapsUnreferencedClients(t *testing.T) {
	router := NewRouter(nil)
	kept := &closableClient{}
	reaped := &closableClient{}
	router.clients[clientCacheKey("opencode", "/tmp/active")] = kept
	router.clients[clientCacheKey("codex", "/tmp/stale")] = reaped

	result, err := router.ReconcileAgentClients(context.Background(), []middleware.AgentClientRef{
		{AgentID: "opencode", WorkspacePath: "/tmp/active"},
	})
	if err != nil {
		t.Fatalf("ReconcileAgentClients: %v", err)
	}
	if kept.closed {
		t.Fatalf("active client must be retained")
	}
	if !reaped.closed {
		t.Fatalf("stale client must be closed")
	}
	if len(result.Reaped) != 1 || result.Reaped[0].AgentID != "codex" {
		t.Fatalf("unexpected reaped refs: %+v", result.Reaped)
	}
	if len(result.Retained) != 1 || result.Retained[0].AgentID != "opencode" {
		t.Fatalf("unexpected retained refs: %+v", result.Retained)
	}
}

type controlClient struct {
	deleted      []string
	cancelled    []string
	closedRemote []string
}

func (c *controlClient) ExecuteTurn(context.Context, middleware.ConversationTurn) (middleware.ConversationResult, error) {
	return middleware.ConversationResult{}, nil
}

func (c *controlClient) Close() error { return nil }

func (c *controlClient) SessionCapabilities() middleware.ConversationSessionCapabilities {
	return middleware.ConversationSessionCapabilities{Delete: true, Cancel: true}
}

func (c *controlClient) ListRemoteSessions(context.Context) ([]middleware.RemoteSessionInfo, error) {
	return nil, nil
}

func (c *controlClient) GetRemoteSession(context.Context, string) (middleware.RemoteSessionInfo, error) {
	return middleware.RemoteSessionInfo{}, nil
}

func (c *controlClient) CancelRemoteSession(_ context.Context, remoteSessionID string) error {
	c.cancelled = append(c.cancelled, remoteSessionID)
	return nil
}

func (c *controlClient) CloseRemoteSession(_ context.Context, remoteSessionID string) error {
	c.closedRemote = append(c.closedRemote, remoteSessionID)
	return nil
}

func (c *controlClient) DeleteRemoteSession(_ context.Context, remoteSessionID string) error {
	c.deleted = append(c.deleted, remoteSessionID)
	return nil
}

func TestRouter_DeleteAgentSessionForWorkspaceUsesExactWorkspaceClient(t *testing.T) {
	router := NewRouter(nil)
	defaultClient := &controlClient{}
	workspaceClient := &controlClient{}
	router.clients[clientCacheKey("opencode", ".")] = defaultClient
	router.clients[clientCacheKey("opencode", "/tmp/ws")] = workspaceClient

	if err := router.DeleteAgentSessionForWorkspace(context.Background(), "opencode", "remote-1", "/tmp/ws"); err != nil {
		t.Fatalf("DeleteAgentSessionForWorkspace: %v", err)
	}
	if len(workspaceClient.deleted) != 1 || workspaceClient.deleted[0] != "remote-1" {
		t.Fatalf("expected workspace client delete, got %+v", workspaceClient.deleted)
	}
	if len(defaultClient.deleted) != 0 {
		t.Fatalf("default client must not receive workspace delete, got %+v", defaultClient.deleted)
	}
}

func TestRouter_CancelAgentSessionForWorkspaceUsesExactWorkspaceClient(t *testing.T) {
	router := NewRouter(nil)
	defaultClient := &controlClient{}
	workspaceClient := &controlClient{}
	router.clients[clientCacheKey("opencode", ".")] = defaultClient
	router.clients[clientCacheKey("opencode", "/tmp/ws")] = workspaceClient

	if err := router.CancelAgentSessionForWorkspace(context.Background(), "opencode", "remote-1", "/tmp/ws"); err != nil {
		t.Fatalf("CancelAgentSessionForWorkspace: %v", err)
	}
	if len(workspaceClient.cancelled) != 1 || workspaceClient.cancelled[0] != "remote-1" {
		t.Fatalf("expected workspace client cancel, got %+v", workspaceClient.cancelled)
	}
	if len(defaultClient.cancelled) != 0 {
		t.Fatalf("default client must not receive workspace cancel, got %+v", defaultClient.cancelled)
	}
}

func TestRouter_CheckAndReconnectEvictsDeadLocalACPWithoutPrewarm(t *testing.T) {
	router := NewRouter(&mockResolver{protocol: "stdio", address: "opencode"})
	client := &deadClosableClient{}
	key := clientCacheKey("opencode", "/tmp/ws")
	router.clients[key] = client

	router.checkAndReconnect()

	if !client.closed {
		t.Fatalf("expected dead local ACP client to be closed")
	}
	if _, ok := router.clients[key]; ok {
		t.Fatalf("expected dead local ACP client to be evicted without replacement")
	}
}

func TestRouter_CancelAgentSessionForWorkspaceDoesNotSpawnFreshLocalACPClient(t *testing.T) {
	router := NewRouter(nil)
	defaultClient := &controlClient{}
	router.clients[clientCacheKey("opencode", ".")] = defaultClient

	err := router.CancelAgentSessionForWorkspace(context.Background(), "opencode", "remote-1", "/tmp/ws")
	if err == nil {
		t.Fatalf("expected no reusable workspace client error")
	}
	if !strings.Contains(err.Error(), noReusableCachedAgentClient) {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defaultClient.cancelled) != 0 {
		t.Fatalf("default client must not receive workspace cancel, got %+v", defaultClient.cancelled)
	}
}
