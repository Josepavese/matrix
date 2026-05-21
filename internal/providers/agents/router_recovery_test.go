package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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

type staticFactory struct {
	client middleware.ConversationClient
}

func (f staticFactory) NewClient(context.Context, middleware.ProtocolEndpoint, middleware.ConversationFactoryDeps) (middleware.ConversationClient, error) {
	return f.client, nil
}

type countingFactory struct {
	calls  int
	client middleware.ConversationClient
}

func (f *countingFactory) NewClient(context.Context, middleware.ProtocolEndpoint, middleware.ConversationFactoryDeps) (middleware.ConversationClient, error) {
	f.calls++
	return f.client, nil
}

type contextBoundFactory struct {
	client *contextBoundClient
}

func (f *contextBoundFactory) NewClient(ctx context.Context, _ middleware.ProtocolEndpoint, _ middleware.ConversationFactoryDeps) (middleware.ConversationClient, error) {
	f.client = &contextBoundClient{ctx: ctx}
	return f.client, nil
}

type contextBoundClient struct {
	ctx context.Context
}

func (c *contextBoundClient) ExecuteTurn(context.Context, middleware.ConversationTurn) (middleware.ConversationResult, error) {
	return middleware.ConversationResult{}, nil
}

func (c *contextBoundClient) Close() error { return nil }

func (c *contextBoundClient) Alive() bool { return c.ctx.Err() == nil }

type promptCancelFactory struct {
	mu      sync.Mutex
	entered chan struct{}
	clients []*promptCancelClient
}

func (f *promptCancelFactory) NewClient(ctx context.Context, _ middleware.ProtocolEndpoint, _ middleware.ConversationFactoryDeps) (middleware.ConversationClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	client := &promptCancelClient{
		ctx:       ctx,
		remoteID:  fmt.Sprintf("remote-%d", len(f.clients)+1),
		block:     len(f.clients) == 0,
		entered:   f.entered,
		output:    "judge-ok",
		createdAt: len(f.clients) + 1,
	}
	f.clients = append(f.clients, client)
	return client, nil
}

type promptCancelClient struct {
	ctx       context.Context
	remoteID  string
	block     bool
	entered   chan struct{}
	output    string
	createdAt int
	mu        sync.Mutex
	closed    bool
	once      sync.Once
}

func (c *promptCancelClient) ExecuteTurn(ctx context.Context, _ middleware.ConversationTurn) (middleware.ConversationResult, error) {
	if c.block {
		c.once.Do(func() { close(c.entered) })
		<-ctx.Done()
		return middleware.ConversationResult{RemoteSessionID: c.remoteID}, ctx.Err()
	}
	return middleware.ConversationResult{Output: c.output, RemoteSessionID: c.remoteID}, nil
}

func (c *promptCancelClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *promptCancelClient) Alive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.ctx.Err() == nil
}

func (c *promptCancelClient) TrackedRemoteSessionIDs() []string {
	return []string{c.remoteID}
}

func (c *promptCancelClient) Closed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
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

type trackedClosableClient struct {
	closableClient
	remoteSessionIDs []string
}

func (c *trackedClosableClient) TrackedRemoteSessionIDs() []string {
	return append([]string(nil), c.remoteSessionIDs...)
}

type trackedDeadClosableClient struct {
	deadClosableClient
	remoteSessionIDs []string
}

func (c *trackedDeadClosableClient) TrackedRemoteSessionIDs() []string {
	return append([]string(nil), c.remoteSessionIDs...)
}

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

func TestRouter_ReapAgentSessionClientPreservesSiblingRemoteTombstones(t *testing.T) {
	router := NewRouter(nil)
	client := &trackedClosableClient{remoteSessionIDs: []string{"parent-remote", "child-remote"}}
	key := clientCacheKey("opencode", "/tmp/ws")
	router.clients[key] = client

	reaped, err := router.ReapAgentSessionClient(context.Background(), "opencode", "child-remote", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentSessionClient child: %v", err)
	}
	if !reaped || !client.closed {
		t.Fatalf("expected child cleanup to reap shared client, reaped=%v closed=%v", reaped, client.closed)
	}
	reaped, err = router.ReapAgentSessionClient(context.Background(), "opencode", "parent-remote", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentSessionClient parent: %v", err)
	}
	if !reaped {
		t.Fatalf("expected parent cleanup to consume sibling remote tombstone")
	}
	reaped, err = router.ReapAgentSessionClient(context.Background(), "opencode", "parent-remote", "/tmp/ws")
	if err != nil {
		t.Fatalf("second ReapAgentSessionClient parent: %v", err)
	}
	if reaped {
		t.Fatalf("expected parent tombstone proof to be single-use")
	}
}

func TestRouter_CachedClientLifetimeSurvivesRequestContextCancel(t *testing.T) {
	router := NewRouter(&mockResolver{protocol: "stdio", address: "opencode"})
	factory := &contextBoundFactory{}
	router.factory[middleware.ProtocolKindACP] = factory

	reqCtx, cancel := context.WithCancel(context.Background())
	client, err := router.getOrCreateClient(reqCtx, "opencode", "/tmp/ws")
	if err != nil {
		t.Fatalf("getOrCreateClient: %v", err)
	}
	if client != factory.client {
		t.Fatalf("expected factory client")
	}
	cancel()
	if !factory.client.Alive() {
		t.Fatalf("cached provider client must not inherit per-request cancellation")
	}
	router.Close()
	if factory.client.Alive() {
		t.Fatalf("router close must cancel provider client lifetime")
	}
}

func TestRouter_PromptContextCancellationDoesNotPoisonNextClient(t *testing.T) {
	router := NewRouter(&mockResolver{protocol: "stdio", address: "opencode"})
	factory := &promptCancelFactory{entered: make(chan struct{})}
	router.factory[middleware.ProtocolKindACP] = factory

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	errCh := make(chan error, 1)
	go func() {
		_, _, _, _, err := router.Route(ctx1, middleware.RouteRequest{
			AgentID:          "opencode",
			LogicalSessionID: "logical-1",
			WorkspacePath:    "/tmp/ws",
			Message:          "block until cancelled",
		})
		errCh <- err
	}()
	select {
	case <-factory.entered:
	case <-time.After(time.Second):
		t.Fatalf("first prompt did not start")
	}
	cancel1()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation from first prompt, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("first prompt did not finish after cancellation")
	}
	first := factory.clients[0]
	if !first.Closed() {
		t.Fatalf("cancelled prompt client must be closed and evicted")
	}
	if _, ok := router.clients[clientCacheKey("opencode", "/tmp/ws")]; ok {
		t.Fatalf("cancelled prompt client must not remain cached")
	}
	reaped, err := router.ReapAgentSessionClient(context.Background(), "opencode", "remote-1", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentSessionClient: %v", err)
	}
	if !reaped {
		t.Fatalf("cancelled prompt eviction must preserve remote-session process proof")
	}

	output, _, _, _, err := router.Route(context.Background(), middleware.RouteRequest{
		AgentID:          "opencode",
		LogicalSessionID: "logical-2",
		WorkspacePath:    "/tmp/ws",
		Message:          "judge this completed task",
	})
	if err != nil {
		t.Fatalf("fresh request after cancellation must not inherit poison: %v", err)
	}
	if output != "judge-ok" {
		t.Fatalf("expected second request output, got %q", output)
	}
	if len(factory.clients) != 2 {
		t.Fatalf("expected a fresh second client, got %d", len(factory.clients))
	}
	if factory.clients[1].ctx.Err() != nil {
		t.Fatalf("second client context must be live, got %v", factory.clients[1].ctx.Err())
	}
}

func TestRouter_ReapAgentSessionClientUsesRecentDeadClientTombstone(t *testing.T) {
	router := NewRouter(&mockResolver{protocol: "stdio", address: "opencode"})
	client := &trackedDeadClosableClient{remoteSessionIDs: []string{"remote-1"}}
	key := clientCacheKey("opencode", "/tmp/ws")
	router.clients[key] = client

	router.checkAndReconnect()

	if !client.closed {
		t.Fatalf("expected dead local ACP client to be closed")
	}
	if _, ok := router.clients[key]; ok {
		t.Fatalf("expected dead local ACP client to be evicted")
	}
	reaped, err := router.ReapAgentSessionClient(context.Background(), "opencode", "remote-1", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentSessionClient: %v", err)
	}
	if !reaped {
		t.Fatalf("expected tombstone to provide process reap proof")
	}
	reaped, err = router.ReapAgentSessionClient(context.Background(), "opencode", "remote-1", "/tmp/ws")
	if err != nil {
		t.Fatalf("second ReapAgentSessionClient: %v", err)
	}
	if reaped {
		t.Fatalf("expected tombstone to be consumed after first proof")
	}
}

func TestRouter_ReapAgentSessionClientRequiresTrackedRemoteSession(t *testing.T) {
	router := NewRouter(&mockResolver{protocol: "stdio", address: "opencode"})
	client := &trackedDeadClosableClient{remoteSessionIDs: []string{"remote-2"}}
	key := clientCacheKey("opencode", "/tmp/ws")
	router.clients[key] = client

	router.checkAndReconnect()

	reaped, err := router.ReapAgentSessionClient(context.Background(), "opencode", "remote-1", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentSessionClient: %v", err)
	}
	if reaped {
		t.Fatalf("must not use tombstone for an unrelated remote session")
	}
}

func TestRouter_LifecycleClientEvictsDeadClientForLaterSessionReap(t *testing.T) {
	router := NewRouter(&mockResolver{protocol: "stdio", address: "opencode"})
	client := &trackedDeadClosableClient{remoteSessionIDs: []string{"remote-1"}}
	key := clientCacheKey("opencode", "/tmp/ws")
	router.clients[key] = client

	err := router.DeleteAgentSessionForWorkspace(context.Background(), "opencode", "remote-1", "/tmp/ws")
	if err == nil || !strings.Contains(err.Error(), noReusableCachedAgentClient) {
		t.Fatalf("expected no reusable lifecycle client error, got %v", err)
	}
	if !client.closed {
		t.Fatalf("dead lifecycle client must be closed")
	}
	if _, ok := router.clients[key]; ok {
		t.Fatalf("dead lifecycle client must be evicted")
	}
	reaped, err := router.ReapAgentSessionClient(context.Background(), "opencode", "remote-1", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentSessionClient: %v", err)
	}
	if !reaped {
		t.Fatalf("lifecycle tombstone must provide later process proof")
	}
}

func TestRouter_ReapAgentSessionClientUsesTombstoneWhenCurrentClientDoesNotTrackSession(t *testing.T) {
	router := NewRouter(nil)
	current := &trackedClosableClient{remoteSessionIDs: []string{"remote-current"}}
	key := clientCacheKey("opencode", "/tmp/ws")
	router.clients[key] = current
	router.clientTombstones[key] = agentClientTombstone{
		reapedAt:         time.Now(),
		remoteSessionIDs: map[string]struct{}{"remote-old": {}},
	}

	reaped, err := router.ReapAgentSessionClient(context.Background(), "opencode", "remote-old", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentSessionClient: %v", err)
	}
	if !reaped {
		t.Fatalf("expected old-session tombstone to provide process proof")
	}
	if current.closed {
		t.Fatalf("must not close current client that does not own the old remote session")
	}
}

func TestRouter_ReapAgentClientDoesNotConsumeExplicitRemoteTombstone(t *testing.T) {
	router := NewRouter(nil)
	key := clientCacheKey("opencode", "/tmp/ws")
	router.clientTombstones[key] = agentClientTombstone{
		reapedAt:         time.Now(),
		remoteSessionIDs: map[string]struct{}{"remote-old": {}},
	}

	reaped, err := router.ReapAgentClient(context.Background(), "opencode", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentClient: %v", err)
	}
	if reaped {
		t.Fatalf("generic workspace reap must not consume remote-session tombstone proof")
	}
	if _, ok := router.clientTombstones[key].remoteSessionIDs["remote-old"]; !ok {
		t.Fatalf("expected remote tombstone to remain available, got %+v", router.clientTombstones[key])
	}

	reaped, err = router.ReapAgentSessionClient(context.Background(), "opencode", "remote-old", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentSessionClient: %v", err)
	}
	if !reaped {
		t.Fatalf("expected explicit remote session cleanup to consume preserved tombstone")
	}
}

func TestRouter_GetOrCreateClientPreservesDeadClientTombstone(t *testing.T) {
	router := NewRouter(&mockResolver{protocol: "stdio", address: "opencode"})
	replacement := &trackedClosableClient{remoteSessionIDs: []string{"remote-new"}}
	router.factory[middleware.ProtocolKindACP] = staticFactory{client: replacement}
	key := clientCacheKey("opencode", "/tmp/ws")
	dead := &trackedDeadClosableClient{remoteSessionIDs: []string{"remote-old"}}
	router.clients[key] = dead

	client, err := router.getOrCreateClient(context.Background(), "opencode", "/tmp/ws")
	if err != nil {
		t.Fatalf("getOrCreateClient: %v", err)
	}
	if client != replacement {
		t.Fatalf("expected replacement client")
	}
	if _, ok := router.clientTombstones[key].remoteSessionIDs["remote-old"]; !ok {
		t.Fatalf("expected dead client remote session tombstone, got %+v", router.clientTombstones[key])
	}
}

func TestRouter_ReapAgentSessionClientDoesNotCloseCurrentUntrackedClient(t *testing.T) {
	router := NewRouter(nil)
	client := &trackedClosableClient{remoteSessionIDs: []string{"remote-2"}}
	key := clientCacheKey("opencode", "/tmp/ws")
	router.clients[key] = client

	reaped, err := router.ReapAgentSessionClient(context.Background(), "opencode", "remote-1", "/tmp/ws")
	if err != nil {
		t.Fatalf("ReapAgentSessionClient: %v", err)
	}
	if reaped || client.closed {
		t.Fatalf("must not close current client for unrelated remote session, reaped=%v closed=%v", reaped, client.closed)
	}
}

func TestRouter_ReconcileAgentClientsReapsUnreferencedClients(t *testing.T) {
	router := NewRouter(nil)
	kept := &closableClient{}
	reaped := &closableClient{}
	router.clients[clientCacheKey("opencode", "/tmp/active")] = kept
	router.clients[clientCacheKey("codex", "/tmp/stale")] = reaped

	result, err := router.ReconcileAgentClients(context.Background(), []middleware.AgentClientRef{
		{
			LogicalSessionID: "active-logical",
			RemoteSessionID:  "active-remote",
			AgentID:          "opencode",
			ProtocolKind:     "acp",
			WorkspacePath:    "/tmp/active",
		},
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
	if result.Retained[0].LogicalSessionID != "active-logical" || result.Retained[0].RemoteSessionID != "active-remote" {
		t.Fatalf("retained refs must preserve session ownership details: %+v", result.Retained)
	}
}

func TestRouter_ReconcileAgentClientsReapsClientThatDoesNotOwnActiveRemoteSession(t *testing.T) {
	router := NewRouter(nil)
	stale := &trackedClosableClient{remoteSessionIDs: []string{"remote-old"}}
	router.clients[clientCacheKey("opencode", "/tmp/active")] = stale

	result, err := router.ReconcileAgentClients(context.Background(), []middleware.AgentClientRef{
		{LogicalSessionID: "active-logical", RemoteSessionID: "remote-current", AgentID: "opencode", WorkspacePath: "/tmp/active"},
	})
	if err != nil {
		t.Fatalf("ReconcileAgentClients: %v", err)
	}
	if !stale.closed {
		t.Fatalf("client that does not track active remote session must be reaped")
	}
	if len(result.Retained) != 0 || len(result.Reaped) != 1 {
		t.Fatalf("unexpected reconcile result: %+v", result)
	}
}

func TestRouter_LifecycleDoesNotSpawnFreshStdioACPClient(t *testing.T) {
	router := NewRouter(&mockResolver{protocol: "stdio", address: "opencode"})
	factory := &countingFactory{client: &closableClient{}}
	router.factory[middleware.ProtocolKindACP] = factory

	err := router.CancelAgentSession(context.Background(), "opencode", "remote-1")
	if err == nil || !strings.Contains(err.Error(), noReusableCachedAgentClient) {
		t.Fatalf("expected no reusable stdio lifecycle client error, got %v", err)
	}
	if factory.calls != 0 {
		t.Fatalf("stdio ACP cleanup lifecycle must not spawn fresh clients, calls=%d", factory.calls)
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
