package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/onboarding"
	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
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
	lastSession    string
	lastRemote     string
	lastMsg        string
	lastStrict     bool
	remote         []middleware.RemoteSessionInfo
	deleted        []string
	canceled       []string
	closed         []string
	deletedWS      []string
	canceledWS     []string
	closedWS       []string
	deleteErr      error
	cancelErr      error
	closeErr       error
	closeOK        bool
	reaped         []string
	reapErr        error
	capErr         error
	capFork        *bool
	forkErr        error
	forked         []middleware.SessionForkRequest
	forkChild      middleware.RemoteSessionInfo
	materialErr    error
	materialized   []middleware.SessionMaterializeRequest
	materialRemote middleware.RemoteSessionInfo
	metadata       middleware.ConversationMetadata
	routeErr       error
}

func (m *mockRouter) Route(_ context.Context, req middleware.RouteRequest) (string, string, []middleware.ToolCall, middleware.ConversationMetadata, error) {
	m.lastSession = req.LogicalSessionID
	m.lastRemote = req.AgentSessionID
	m.lastMsg = req.Message
	m.lastStrict = req.StrictSession
	return "Ok", req.AgentSessionID, nil, m.metadata, m.routeErr
}

func (m *mockRouter) ListAgentSessions(_ context.Context, _ string) ([]middleware.RemoteSessionInfo, middleware.ConversationSessionCapabilities, error) {
	return m.remote, middleware.ConversationSessionCapabilities{List: true, Load: true, Cancel: true, Close: m.closeOK, Delete: true}, nil
}

func (m *mockRouter) GetAgentSession(_ context.Context, _ string, remoteSessionID string) (middleware.RemoteSessionInfo, error) {
	for _, session := range m.remote {
		if session.RemoteSessionID == remoteSessionID || session.DisplayID == remoteSessionID {
			return session, nil
		}
	}
	return middleware.RemoteSessionInfo{}, nil
}

func (m *mockRouter) DeleteAgentSession(_ context.Context, _ string, remoteSessionID string) error {
	m.deleted = append(m.deleted, remoteSessionID)
	return m.deleteErr
}

func (m *mockRouter) CancelAgentSession(_ context.Context, _ string, remoteSessionID string) error {
	m.canceled = append(m.canceled, remoteSessionID)
	return m.cancelErr
}

func (m *mockRouter) CloseAgentSession(_ context.Context, _ string, remoteSessionID string) error {
	if !m.closeOK {
		return fmt.Errorf("agent router does not expose remote session close")
	}
	m.closed = append(m.closed, remoteSessionID)
	return m.closeErr
}

func (m *mockRouter) DeleteAgentSessionForWorkspace(ctx context.Context, agentID string, remoteSessionID string, workspacePath string) error {
	m.deletedWS = append(m.deletedWS, remoteSessionID+"|"+workspacePath)
	return m.DeleteAgentSession(ctx, agentID, remoteSessionID)
}

func (m *mockRouter) CancelAgentSessionForWorkspace(ctx context.Context, agentID string, remoteSessionID string, workspacePath string) error {
	m.canceledWS = append(m.canceledWS, remoteSessionID+"|"+workspacePath)
	return m.CancelAgentSession(ctx, agentID, remoteSessionID)
}

func (m *mockRouter) CloseAgentSessionForWorkspace(ctx context.Context, agentID string, remoteSessionID string, workspacePath string) error {
	if !m.closeOK {
		return fmt.Errorf("agent router does not expose remote session close")
	}
	m.closedWS = append(m.closedWS, remoteSessionID+"|"+workspacePath)
	return m.CloseAgentSession(ctx, agentID, remoteSessionID)
}

func (m *mockRouter) ReapAgentClient(_ context.Context, agentID string, workspacePath string) (bool, error) {
	m.reaped = append(m.reaped, agentID+"|"+workspacePath)
	if m.reapErr != nil {
		return false, m.reapErr
	}
	return true, nil
}

func (m *mockRouter) AgentCapabilities(_ context.Context, agentID string) (middleware.ProviderCapabilityReport, error) {
	if m.capErr != nil {
		return middleware.ProviderCapabilityReport{}, m.capErr
	}
	forkSupported := true
	if m.capFork != nil {
		forkSupported = *m.capFork
	}
	return middleware.ProviderCapabilityReport{
		AgentID:      agentID,
		ProtocolKind: middleware.ProtocolKindACP,
		Session: map[string]middleware.CapabilityDescriptor{
			"fork": {Name: "fork", Supported: forkSupported, Status: "supported", Stability: "draft", Source: "test"},
		},
	}, nil
}

func (m *mockRouter) MaterializeAgentSession(_ context.Context, _ string, req middleware.SessionMaterializeRequest) (middleware.RemoteSessionInfo, middleware.ConversationMetadata, error) {
	m.materialized = append(m.materialized, req)
	if m.materialErr != nil {
		return middleware.RemoteSessionInfo{}, middleware.ConversationMetadata{}, m.materialErr
	}
	if strings.TrimSpace(m.materialRemote.RemoteSessionID) != "" {
		return m.materialRemote, m.metadata, nil
	}
	return middleware.RemoteSessionInfo{
		RemoteSessionID: "materialized-parent-remote",
		DisplayID:       "materialized-parent-remote",
		ProtocolKind:    middleware.ProtocolKindACP,
		CanResume:       true,
	}, m.metadata, nil
}

func (m *mockRouter) ForkAgentSession(_ context.Context, _ string, req middleware.SessionForkRequest) (middleware.RemoteSessionInfo, error) {
	m.forked = append(m.forked, req)
	if m.forkErr != nil {
		return middleware.RemoteSessionInfo{}, m.forkErr
	}
	if strings.TrimSpace(m.forkChild.RemoteSessionID) != "" {
		return m.forkChild, nil
	}
	return middleware.RemoteSessionInfo{
		RemoteSessionID: "fork-child-remote",
		DisplayID:       "fork-child-remote",
		ProtocolKind:    middleware.ProtocolKindACP,
		CanResume:       true,
	}, nil
}

type mockResolver struct{}
type mockLocalizer struct{}

func (m *mockResolver) GetAgentEndpoint(_ string) (middleware.ProtocolEndpoint, error) {
	return middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   "mock-agent",
	}, nil
}

func (m *mockLocalizer) GetString(_ string, key string) string {
	return key
}

func newTestWizard(storage middleware.Storage) *onboarding.Wizard {
	return onboarding.NewWizard(onboarding.WizardDependencies{
		Storage:   storage,
		Localizer: &mockLocalizer{},
	})
}

func TestSessionManager_GetOrCreate(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{
		metadata: middleware.ConversationMetadata{
			Title:     "Remote Session Title",
			UpdatedAt: "2026-04-14T20:00:00Z",
			Status:    "active",
			Meta: map[string]interface{}{
				"source": "agent",
			},
		},
	}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

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
	router := &mockRouter{
		metadata: middleware.ConversationMetadata{
			Title:     "Remote Session Title",
			UpdatedAt: "2026-04-14T20:00:00Z",
			Status:    "active",
			Meta: map[string]interface{}{
				"source": "agent",
			},
		},
	}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

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

func TestSessionManager_DoesNotParseCommandsForNonInteractiveRuns(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	res, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:      "run_1",
		AgentID:        "codex",
		Input:          "/status",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("RouteConversation failed: %v", err)
	}
	if res != "Ok" {
		t.Fatalf("expected agent response, got %q", res)
	}
	if router.lastMsg != "/status" {
		t.Fatalf("expected /status to reach agent, got %q", router.lastMsg)
	}
}

func TestSessionManager_SlashCommandsRequireExactToken(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	res, err := mgr.Route(context.Background(), "chat_1", "codex", "/statusfoo", nil)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if res != "Ok" {
		t.Fatalf("expected agent response, got %q", res)
	}
	if router.lastMsg != "/statusfoo" {
		t.Fatalf("expected /statusfoo to reach agent, got %q", router.lastMsg)
	}
}

func TestSessionManager_MirrorsRemoteSessionMetadata(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{
		metadata: middleware.ConversationMetadata{
			Title:     "Remote Session Title",
			UpdatedAt: "2026-04-14T20:00:00Z",
			Status:    "active",
			Meta: map[string]interface{}{
				"source": "agent",
			},
		},
	}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	mgr.SetEndpointResolver(&mockResolver{})

	sessionID, err := mgr.GetOrCreateSession("mirror-ch", "gemini")
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}
	if _, err := mgr.Route(context.Background(), "mirror-ch", "gemini", "Hello", nil); err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	raw, err := storage.Get("session.meta." + sessionID)
	if err != nil {
		t.Fatalf("Get session meta: %v", err)
	}
	var meta SessionMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("Unmarshal session meta: %v", err)
	}
	if meta.ProtocolKind != "acp" {
		t.Fatalf("expected protocol_kind=acp, got %q", meta.ProtocolKind)
	}
	if meta.MirrorStatus != "mirrored" {
		t.Fatalf("expected mirror_status=mirrored, got %q", meta.MirrorStatus)
	}
	if meta.RemoteTitle != "Remote Session Title" {
		t.Fatalf("expected remote_title to mirror metadata, got %q", meta.RemoteTitle)
	}
	if meta.RemoteStatus != "active" {
		t.Fatalf("expected remote_status=active, got %q", meta.RemoteStatus)
	}
	if meta.RemoteMeta["source"] != "agent" {
		t.Fatalf("expected remote_meta[source]=agent, got %+v", meta.RemoteMeta)
	}
	if meta.LastSyncedAt.IsZero() {
		t.Fatal("expected last_synced_at to be populated")
	}
	if meta.RemoteUpdatedAt.IsZero() {
		t.Fatal("expected remote_updated_at from metadata")
	}
}

func TestSessionManager_Attach(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

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

func TestSessionManager_SessionListIncludesRemoteSessions(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{
		remote: []middleware.RemoteSessionInfo{
			{RemoteSessionID: "acp-remote-1", DisplayID: "acp-remote-1", Title: "Remote Draft", ProtocolKind: middleware.ProtocolKindACP},
		},
	}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	if _, err := mgr.GetOrCreateSession("telegram_1", "codex"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	got, err := mgr.Route(context.Background(), "telegram_1", "codex", "/session list", nil)
	if err != nil {
		t.Fatalf("Route list failed: %v", err)
	}
	if got == "" || !contains(got, "Remote sessions:") || !contains(got, "acp-remote-1") {
		t.Fatalf("expected remote sessions in list, got %q", got)
	}
}

func TestSessionManager_TypedStatusNewAndName(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	newResult, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID: "typed_1",
		Action:    "new",
		Target:    "claude",
	})
	if err != nil {
		t.Fatalf("HandleSessionActionTyped(new): %v", err)
	}
	if newResult.Action != "new" || newResult.Session == nil || newResult.Session.AgentID != "claude" {
		t.Fatalf("unexpected new result: %+v", newResult)
	}

	nameResult, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID: "typed_1",
		Action:    "name",
		Target:    "bugfix",
	})
	if err != nil {
		t.Fatalf("HandleSessionActionTyped(name): %v", err)
	}
	if nameResult.Action != "name" || nameResult.Session == nil || nameResult.Session.Alias != "bugfix" {
		t.Fatalf("unexpected name result: %+v", nameResult)
	}

	statusResult, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID: "typed_1",
		Action:    "status",
	})
	if err != nil {
		t.Fatalf("HandleSessionActionTyped(status): %v", err)
	}
	if statusResult.Action != "status" || statusResult.Session == nil || statusResult.Session.LogicalSessionID != newResult.ActiveSessionID {
		t.Fatalf("unexpected status result: %+v", statusResult)
	}
	if statusResult.Session.Alias != "bugfix" {
		t.Fatalf("expected alias to be mirrored in status result, got %+v", statusResult.Session)
	}
}

func TestSessionManager_RouteConversationBindsWorkspace(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:             "billing-api",
		Name:           "billing-api",
		RootPath:       "/tmp/billing-api",
		DefaultAgentID: "claude",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:   "telegram_workspace_1",
		WorkspaceID: "billing-api",
		Input:       "hello",
	}); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}

	state, err := mgr.getChannelState("telegram_workspace_1")
	if err != nil {
		t.Fatalf("getChannelState: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(state.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	if meta.WorkspaceID != "billing-api" {
		t.Fatalf("expected workspace_id=billing-api, got %q", meta.WorkspaceID)
	}
	if meta.WorkspacePath != "/tmp/billing-api" {
		t.Fatalf("expected workspace_path=/tmp/billing-api, got %q", meta.WorkspacePath)
	}
	if meta.AgentID != "claude" {
		t.Fatalf("expected workspace default agent claude, got %q", meta.AgentID)
	}
	if state.PreferredWorkspaceID != "billing-api" {
		t.Fatalf("expected preferred workspace to be updated, got %q", state.PreferredWorkspaceID)
	}
	events, err := workspace.LoadTimeline(storage, "billing-api", 10)
	if err != nil {
		t.Fatalf("LoadTimeline: %v", err)
	}
	foundCreated := false
	foundDecision := false
	for _, event := range events {
		switch event.Type {
		case "session.created":
			foundCreated = true
		case "decision.recorded":
			foundDecision = true
		}
	}
	if !foundCreated || !foundDecision {
		t.Fatalf("expected session.created and decision.recorded in workspace timeline, got %+v", events)
	}
}

func TestSessionManager_ReusesWorkspaceSessionAcrossChannels(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:       "billing-api",
		Name:     "billing-api",
		RootPath: "/tmp/billing-api",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:   "channel_a",
		AgentID:     "codex",
		WorkspaceID: "billing-api",
		Input:       "hello",
	}); err != nil {
		t.Fatalf("RouteConversation(channel_a): %v", err)
	}
	stateA, err := mgr.getChannelState("channel_a")
	if err != nil {
		t.Fatalf("getChannelState(channel_a): %v", err)
	}
	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:   "channel_b",
		AgentID:     "codex",
		WorkspaceID: "billing-api",
		Input:       "continue",
	}); err != nil {
		t.Fatalf("RouteConversation(channel_b): %v", err)
	}
	stateB, err := mgr.getChannelState("channel_b")
	if err != nil {
		t.Fatalf("getChannelState(channel_b): %v", err)
	}
	if stateA.ActiveSessionID != stateB.ActiveSessionID {
		t.Fatalf("expected same session to be reused for workspace, got %s vs %s", stateA.ActiveSessionID, stateB.ActiveSessionID)
	}
}

func TestSessionManager_WorkspaceCommands(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:             "billing-api",
		Name:           "billing-api",
		RootPath:       "/tmp/billing-api",
		DefaultAgentID: "claude",
	}); err != nil {
		t.Fatalf("SaveMeta billing-api: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:       "frontend",
		Name:     "frontend",
		RootPath: "/tmp/frontend",
	}); err != nil {
		t.Fatalf("SaveMeta frontend: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID: "workspace_cmd_ch",
		AgentID:   "codex",
		Input:     "hello",
	}); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}

	listOut, err := mgr.Route(context.Background(), "workspace_cmd_ch", "codex", "/workspace list", nil)
	if err != nil {
		t.Fatalf("/workspace list failed: %v", err)
	}
	if !contains(listOut, "billing-api") || !contains(listOut, "frontend") {
		t.Fatalf("expected configured workspaces in list, got %q", listOut)
	}

	bindOut, err := mgr.Route(context.Background(), "workspace_cmd_ch", "codex", "/workspace bind billing-api", nil)
	if err != nil {
		t.Fatalf("/workspace bind failed: %v", err)
	}
	if !contains(bindOut, "billing-api") {
		t.Fatalf("expected bind response to mention workspace, got %q", bindOut)
	}
	state, err := mgr.getChannelState("workspace_cmd_ch")
	if err != nil {
		t.Fatalf("getChannelState: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(state.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	if meta.WorkspaceID != "billing-api" {
		t.Fatalf("expected active session bound to billing-api, got %q", meta.WorkspaceID)
	}

	switchOut, err := mgr.Route(context.Background(), "workspace_switch_ch", "codex", "/workspace switch billing-api", nil)
	if err != nil {
		t.Fatalf("/workspace switch failed: %v", err)
	}
	if !contains(switchOut, "billing-api") {
		t.Fatalf("expected switch response to mention workspace, got %q", switchOut)
	}
	switchState, err := mgr.getChannelState("workspace_switch_ch")
	if err != nil {
		t.Fatalf("getChannelState(switch): %v", err)
	}
	if switchState.ActiveSessionID != state.ActiveSessionID {
		t.Fatalf("expected switch to reuse workspace session, got %s vs %s", switchState.ActiveSessionID, state.ActiveSessionID)
	}
}

func TestSessionManager_HandoffCreatesTransferPacket(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:              "billing-api",
		Name:            "billing-api",
		RootPath:        "/tmp/billing-api",
		DefaultAgentID:  "codex",
		ReviewerAgentID: "claude",
		DefaultMode:     "implementation",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:   "handoff_ch",
		AgentID:     "codex",
		WorkspaceID: "billing-api",
		Input:       "Implement the billing retry fix",
	}); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}

	result, err := mgr.HandleIntentTyped(context.Background(), middleware.IntentActionRequest{
		ChannelID: "handoff_ch",
		Intent:    "handoff",
		AgentID:   "claude",
		Note:      "Review the current retry patch.",
	})
	if err != nil {
		t.Fatalf("HandleIntentTyped(handoff): %v", err)
	}
	if result.Intent != "handoff" || result.Session == nil || result.Session.AgentID != "claude" {
		t.Fatalf("unexpected handoff result: %+v", result)
	}
	if result.Handoff == nil || result.Handoff.FromAgentID != "codex" || result.Handoff.ToAgentID != "claude" {
		t.Fatalf("unexpected handoff packet: %+v", result.Handoff)
	}

	state, err := mgr.getChannelState("handoff_ch")
	if err != nil {
		t.Fatalf("getChannelState: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(state.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	if meta.PendingHandoff == nil {
		t.Fatal("expected pending handoff on target session")
	}
	if !contains(meta.PendingHandoff.Summary, "Review the current retry patch.") {
		t.Fatalf("expected handoff summary to include operator note, got %q", meta.PendingHandoff.Summary)
	}
	events, err := workspace.LoadTimeline(storage, "billing-api", 10)
	if err != nil {
		t.Fatalf("LoadTimeline: %v", err)
	}
	foundIntent := false
	foundHandoff := false
	for _, event := range events {
		switch event.Type {
		case "intent.handoff", "intent.review":
			foundIntent = true
		case "handoff.created":
			foundHandoff = true
		}
	}
	if !foundIntent || !foundHandoff {
		t.Fatalf("expected mode and handoff events, got %+v", events)
	}
}

func TestSessionManager_RouteAppliesPendingHandoffPrompt(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:       "billing-api",
		Name:     "billing-api",
		RootPath: "/tmp/billing-api",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:   "handoff_route_ch",
		AgentID:     "codex",
		WorkspaceID: "billing-api",
		Input:       "Start implementation",
	}); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}
	if _, err := mgr.HandleIntentTyped(context.Background(), middleware.IntentActionRequest{
		ChannelID: "handoff_route_ch",
		Intent:    "handoff",
		AgentID:   "claude",
		Note:      "Review the implementation plan.",
	}); err != nil {
		t.Fatalf("HandleIntentTyped(handoff): %v", err)
	}
	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:   "handoff_route_ch",
		WorkspaceID: "billing-api",
		Input:       "Check the current patch.",
	}); err != nil {
		t.Fatalf("RouteConversation after handoff: %v", err)
	}
	if !contains(router.lastMsg, "[Matrix handoff context]") {
		t.Fatalf("expected handoff context to be injected, got %q", router.lastMsg)
	}
	if !contains(router.lastMsg, "Review the implementation plan.") {
		t.Fatalf("expected handoff note in injected prompt, got %q", router.lastMsg)
	}

	state, err := mgr.getChannelState("handoff_route_ch")
	if err != nil {
		t.Fatalf("getChannelState: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(state.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	if meta.PendingHandoff != nil {
		t.Fatal("expected pending handoff to be cleared after successful routed turn")
	}
	if meta.LastHandoff == nil || meta.LastHandoff.ToAgentID != "claude" {
		t.Fatalf("expected last handoff to remain mirrored, got %+v", meta.LastHandoff)
	}
	events, err := workspace.LoadTimeline(storage, "billing-api", 10)
	if err != nil {
		t.Fatalf("LoadTimeline: %v", err)
	}
	foundApplied := false
	for _, event := range events {
		if event.Type == "handoff.applied" {
			foundApplied = true
			break
		}
	}
	if !foundApplied {
		t.Fatalf("expected handoff.applied event, got %+v", events)
	}
}

func TestSessionManager_CancelWritesTimelineEvent(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:       "billing-api",
		Name:     "billing-api",
		RootPath: "/tmp/billing-api",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:   "cancel_timeline_ch",
		AgentID:     "codex",
		WorkspaceID: "billing-api",
		Input:       "Start implementation",
	}); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}
	state, err := mgr.getChannelState("cancel_timeline_ch")
	if err != nil {
		t.Fatalf("getChannelState: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(state.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	meta.AgentSessionID = "remote-1"
	if err := mgr.saveSessionMeta(meta); err != nil {
		t.Fatalf("saveSessionMeta: %v", err)
	}
	if _, err := mgr.handleSessionCancelTyped(context.Background(), "cancel_timeline_ch", "en", ""); err != nil {
		t.Fatalf("handleSessionCancelTyped: %v", err)
	}
	events, err := workspace.LoadTimeline(storage, "billing-api", 10)
	if err != nil {
		t.Fatalf("LoadTimeline: %v", err)
	}
	if len(events) == 0 || events[0].Type != "session.canceled" {
		t.Fatalf("expected session.canceled event, got %+v", events)
	}
}

func TestSessionManager_WorkspaceReadTyped(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:             "billing-api",
		Name:           "billing-api",
		RootPath:       "/tmp/billing-api",
		DefaultAgentID: "claude",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:   "read_ws_ch",
		WorkspaceID: "billing-api",
		Input:       "hello",
	}); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}

	stateResult, err := mgr.HandleWorkspaceReadTyped(context.Background(), middleware.WorkspaceReadRequest{
		ChannelID: "read_ws_ch",
		Action:    "state",
	})
	if err != nil {
		t.Fatalf("HandleWorkspaceReadTyped(state): %v", err)
	}
	if stateResult.State == nil || stateResult.State.WorkspaceID != "billing-api" {
		t.Fatalf("unexpected workspace state result: %+v", stateResult)
	}
	if stateResult.State.LastDecision == nil || stateResult.State.LastDecision.SelectedAgentID != "claude" {
		t.Fatalf("expected last decision to be mirrored in state result, got %+v", stateResult.State)
	}

	timelineResult, err := mgr.HandleWorkspaceReadTyped(context.Background(), middleware.WorkspaceReadRequest{
		ChannelID: "read_ws_ch",
		Action:    "timeline",
	})
	if err != nil {
		t.Fatalf("HandleWorkspaceReadTyped(timeline): %v", err)
	}
	if len(timelineResult.Timeline) == 0 || timelineResult.Timeline[0].Type == "" {
		t.Fatalf("unexpected workspace timeline result: %+v", timelineResult)
	}

	decisionsResult, err := mgr.HandleWorkspaceReadTyped(context.Background(), middleware.WorkspaceReadRequest{
		ChannelID: "read_ws_ch",
		Action:    "decisions",
	})
	if err != nil {
		t.Fatalf("HandleWorkspaceReadTyped(decisions): %v", err)
	}
	if len(decisionsResult.Decisions) == 0 || decisionsResult.Decisions[0].SelectedAgentID != "claude" {
		t.Fatalf("expected workspace decisions, got %+v", decisionsResult)
	}

	memoryResult, err := mgr.HandleWorkspaceReadTyped(context.Background(), middleware.WorkspaceReadRequest{
		ChannelID: "read_ws_ch",
		Action:    "memory",
	})
	if err != nil {
		t.Fatalf("HandleWorkspaceReadTyped(memory): %v", err)
	}
	if len(memoryResult.Memory) == 0 {
		t.Fatalf("expected workspace memory, got %+v", memoryResult)
	}
}

func TestSessionManager_NowAndTimelineCommands(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:       "billing-api",
		Name:     "billing-api",
		RootPath: "/tmp/billing-api",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:   "timeline_cmd_ch",
		AgentID:     "codex",
		WorkspaceID: "billing-api",
		Input:       "hello",
	}); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}
	nowOut, err := mgr.Route(context.Background(), "timeline_cmd_ch", "codex", "/now", nil)
	if err != nil {
		t.Fatalf("/now failed: %v", err)
	}
	if !contains(nowOut, "Workspace: billing-api") {
		t.Fatalf("unexpected /now output: %q", nowOut)
	}
	if !contains(nowOut, "Decision:") {
		t.Fatalf("expected decision trace in /now output, got %q", nowOut)
	}
	timelineOut, err := mgr.Route(context.Background(), "timeline_cmd_ch", "codex", "/timeline", nil)
	if err != nil {
		t.Fatalf("/timeline failed: %v", err)
	}
	if !contains(timelineOut, "Workspace timeline: billing-api") {
		t.Fatalf("unexpected /timeline output: %q", timelineOut)
	}
	if !contains(timelineOut, "created session for codex") {
		t.Fatalf("expected friendly timeline wording, got %q", timelineOut)
	}
	decisionsOut, err := mgr.Route(context.Background(), "timeline_cmd_ch", "codex", "/decisions", nil)
	if err != nil {
		t.Fatalf("/decisions failed: %v", err)
	}
	if !contains(decisionsOut, "Workspace decisions: billing-api") {
		t.Fatalf("unexpected /decisions output: %q", decisionsOut)
	}
	whyOut, err := mgr.Route(context.Background(), "timeline_cmd_ch", "codex", "/why", nil)
	if err != nil {
		t.Fatalf("/why failed: %v", err)
	}
	if !contains(whyOut, "Workspace decisions: billing-api") {
		t.Fatalf("unexpected /why output: %q", whyOut)
	}
	memoryOut, err := mgr.Route(context.Background(), "timeline_cmd_ch", "codex", "/memory", nil)
	if err != nil {
		t.Fatalf("/memory failed: %v", err)
	}
	if !contains(memoryOut, "Workspace memory: billing-api") {
		t.Fatalf("unexpected /memory output: %q", memoryOut)
	}
	snapshotOut, err := mgr.Route(context.Background(), "timeline_cmd_ch", "codex", "/snapshot review-ready", nil)
	if err != nil {
		t.Fatalf("/snapshot failed: %v", err)
	}
	if !contains(snapshotOut, "Created snapshot") {
		t.Fatalf("unexpected /snapshot output: %q", snapshotOut)
	}
	snapshotsOut, err := mgr.Route(context.Background(), "timeline_cmd_ch", "codex", "/snapshots", nil)
	if err != nil {
		t.Fatalf("/snapshots failed: %v", err)
	}
	if !contains(snapshotsOut, "Workspace snapshots: billing-api") {
		t.Fatalf("unexpected /snapshots output: %q", snapshotsOut)
	}
}

func TestSessionManager_TypedWorkspaceActions(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:             "billing-api",
		Name:           "billing-api",
		RootPath:       "/tmp/billing-api",
		DefaultAgentID: "claude",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	listResult, err := mgr.HandleWorkspaceActionTyped(context.Background(), middleware.WorkspaceActionRequest{
		ChannelID: "typed_ws",
		Action:    "list",
	})
	if err != nil {
		t.Fatalf("HandleWorkspaceActionTyped(list): %v", err)
	}
	if len(listResult.Workspaces) != 1 || listResult.Workspaces[0].ID != "billing-api" {
		t.Fatalf("unexpected workspace list result: %+v", listResult)
	}

	switchResult, err := mgr.HandleWorkspaceActionTyped(context.Background(), middleware.WorkspaceActionRequest{
		ChannelID: "typed_ws",
		Action:    "switch",
		Target:    "billing-api",
	})
	if err != nil {
		t.Fatalf("HandleWorkspaceActionTyped(switch): %v", err)
	}
	if switchResult.Workspace == nil || switchResult.Workspace.ID != "billing-api" {
		t.Fatalf("unexpected workspace switch result: %+v", switchResult)
	}
	if switchResult.Session == nil || switchResult.Session.WorkspaceID != "billing-api" {
		t.Fatalf("expected switched session to be bound to workspace, got %+v", switchResult.Session)
	}
}

func TestSessionManager_WorkspaceAliases(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:             "billing-api",
		Name:           "billing-api",
		RootPath:       "/tmp/billing-api",
		DefaultAgentID: "claude",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	listOut, err := mgr.Route(context.Background(), "alias_ch", "codex", "/workspaces", nil)
	if err != nil {
		t.Fatalf("/workspaces failed: %v", err)
	}
	if !contains(listOut, "billing-api") {
		t.Fatalf("expected /workspaces to list billing-api, got %q", listOut)
	}

	useOut, err := mgr.Route(context.Background(), "alias_ch", "codex", "/use billing-api", nil)
	if err != nil {
		t.Fatalf("/use failed: %v", err)
	}
	if !contains(useOut, "billing-api") {
		t.Fatalf("expected /use to mention billing-api, got %q", useOut)
	}
	state, err := mgr.getChannelState("alias_ch")
	if err != nil {
		t.Fatalf("getChannelState: %v", err)
	}
	if state.PreferredWorkspaceID != "billing-api" {
		t.Fatalf("expected preferred workspace billing-api, got %q", state.PreferredWorkspaceID)
	}
}

func TestSessionManager_StatusAlias(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:             "billing-api",
		Name:           "billing-api",
		RootPath:       "/tmp/billing-api",
		DefaultAgentID: "claude",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{
		metadata: middleware.ConversationMetadata{
			Title:  "Invoice fix",
			Status: "active",
		},
	}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:   "status_ch",
		WorkspaceID: "billing-api",
		Input:       "hello",
	}); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}

	out, err := mgr.Route(context.Background(), "status_ch", "claude", "/status", nil)
	if err != nil {
		t.Fatalf("/status failed: %v", err)
	}
	if !contains(out, "billing-api") || !contains(out, "claude") || !contains(out, "Invoice fix") {
		t.Fatalf("expected high-level status summary, got %q", out)
	}
}

func TestSessionManager_HighLevelIntents(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{
		ID:              "billing-api",
		Name:            "billing-api",
		RootPath:        "/tmp/billing-api",
		DefaultAgentID:  "codex",
		ReviewerAgentID: "claude",
	}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	if _, err := mgr.RouteConversation(context.Background(), middleware.ConversationRequest{
		ChannelID:   "intent_ch",
		WorkspaceID: "billing-api",
		AgentID:     "codex",
		Input:       "hello",
	}); err != nil {
		t.Fatalf("RouteConversation: %v", err)
	}

	continueOut, err := mgr.Route(context.Background(), "intent_ch", "codex", "/continue", nil)
	if err != nil {
		t.Fatalf("/continue failed: %v", err)
	}
	if !contains(continueOut, "billing-api") || !contains(continueOut, "codex") {
		t.Fatalf("unexpected continue output: %q", continueOut)
	}

	reviewOut, err := mgr.Route(context.Background(), "intent_ch", "codex", "/review", nil)
	if err != nil {
		t.Fatalf("/review failed: %v", err)
	}
	if !contains(reviewOut, "billing-api") || !contains(reviewOut, "claude") {
		t.Fatalf("unexpected review output: %q", reviewOut)
	}
	state, err := mgr.getChannelState("intent_ch")
	if err != nil {
		t.Fatalf("getChannelState: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(state.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	if meta.AgentID != "claude" {
		t.Fatalf("expected review intent to switch to reviewer agent, got %q", meta.AgentID)
	}
	if meta.Mode != modeReview {
		t.Fatalf("expected review mode, got %q", meta.Mode)
	}

	resumeOut, err := mgr.Route(context.Background(), "intent_ch", "codex", "/resume billing-api", nil)
	if err != nil {
		t.Fatalf("/resume failed: %v", err)
	}
	if !contains(resumeOut, "billing-api") {
		t.Fatalf("unexpected resume output: %q", resumeOut)
	}

	explainOut, err := mgr.Route(context.Background(), "intent_ch", "codex", "/explain", nil)
	if err != nil {
		t.Fatalf("/explain failed: %v", err)
	}
	if !contains(explainOut, "billing-api") || !contains(explainOut, "claude") {
		t.Fatalf("unexpected explain output: %q", explainOut)
	}
	meta, found, err = mgr.loadSessionMeta(state.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta after explain: err=%v found=%v", err, found)
	}
	if meta.Mode != modeExplain {
		t.Fatalf("expected explain mode, got %q", meta.Mode)
	}
}

func TestSessionManager_SessionSwitchImportsRemoteSession(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{
		remote: []middleware.RemoteSessionInfo{
			{RemoteSessionID: "acp-remote-2", DisplayID: "acp-remote-2", Title: "Imported Remote", ProtocolKind: middleware.ProtocolKindACP},
		},
	}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	if _, err := mgr.GetOrCreateSession("telegram_2", "codex"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	got, err := mgr.Route(context.Background(), "telegram_2", "codex", "/session switch acp-remote-2", nil)
	if err != nil {
		t.Fatalf("Route switch failed: %v", err)
	}
	if !contains(got, "Switched to remote acp session") {
		t.Fatalf("expected remote switch response, got %q", got)
	}

	state, err := mgr.getChannelState("telegram_2")
	if err != nil {
		t.Fatalf("getChannelState: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(state.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("expected imported session meta, err=%v found=%v", err, found)
	}
	if meta.AgentSessionID != "acp-remote-2" {
		t.Fatalf("expected imported remote session id, got %q", meta.AgentSessionID)
	}
	if meta.RemoteTitle != "Imported Remote" {
		t.Fatalf("expected imported remote title, got %q", meta.RemoteTitle)
	}
}

func TestSessionManager_SessionDeleteRemovesMirrorAndCallsRemoteDelete(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	sessionID, err := mgr.GetOrCreateSession("telegram_3", "codex")
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(sessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	meta.AgentSessionID = "acp-remote-3"
	if err := mgr.saveSessionMeta(meta); err != nil {
		t.Fatalf("saveSessionMeta: %v", err)
	}

	got, err := mgr.Route(context.Background(), "telegram_3", "codex", "/session delete", nil)
	if err != nil {
		t.Fatalf("Route delete failed: %v", err)
	}
	if !contains(got, "Deleted session:") {
		t.Fatalf("expected delete response, got %q", got)
	}
	if len(router.deleted) != 1 || router.deleted[0] != "acp-remote-3" {
		t.Fatalf("expected remote delete call, got %+v", router.deleted)
	}
	if len(router.reaped) != 1 || router.reaped[0] != "codex|" {
		t.Fatalf("expected unreferenced agent client reap, got %+v", router.reaped)
	}
	raw, _ := storage.Get("session.meta." + sessionID)
	if len(raw) != 0 {
		t.Fatalf("expected session meta deleted, got %q", string(raw))
	}
}

func TestSessionManager_DeleteRetainsSharedAgentClient(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	first, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "channel-a",
		Action:        "new",
		Target:        "codex",
		WorkspacePath: "/tmp/shared-ws",
	})
	if err != nil {
		t.Fatalf("new first: %v", err)
	}
	if _, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "channel-b",
		Action:        "new",
		Target:        "codex",
		WorkspacePath: "/tmp/shared-ws",
	}); err != nil {
		t.Fatalf("new second: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(first.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("load first: err=%v found=%v", err, found)
	}
	meta.AgentSessionID = "remote-shared-a"
	if err := mgr.saveSessionMeta(meta); err != nil {
		t.Fatalf("save first: %v", err)
	}

	result, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID: "channel-a",
		Action:    "delete",
		Target:    meta.ID,
	})
	if err != nil {
		t.Fatalf("delete first: %v", err)
	}
	if result.Cleanup == nil {
		t.Fatalf("expected cleanup proof")
	}
	if !result.Cleanup.ProcessRetained || !result.Cleanup.ProcessRetentionAllowed {
		t.Fatalf("expected allowed process retention for shared client, got %+v", result.Cleanup)
	}
	if result.Cleanup.ProcessReapAttempted || len(router.reaped) != 0 {
		t.Fatalf("shared client must not be reaped, proof=%+v reaped=%+v", result.Cleanup, router.reaped)
	}
	if !result.Cleanup.Clean {
		t.Fatalf("allowed shared retention should still be clean, got %+v", result.Cleanup)
	}
}

func TestSessionManager_CapabilitiesUnknownAgentReturnsTypedError(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{capErr: fmt.Errorf("resolve failed: %w", middleware.ErrAgentNotFound)}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	result, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID: "capability-ch",
		Action:    "capabilities",
		Target:    "codex-acp",
	})
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if !result.Unsupported || result.Error == nil {
		t.Fatalf("expected typed unsupported error, got %+v", result)
	}
	if result.Error.Code != "agent_not_found" || result.Error.Target != "codex-acp" {
		t.Fatalf("unexpected typed error: %+v", result.Error)
	}
}

func TestSessionManager_ForkCanPreserveActiveParent(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	parent, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "fork-ch",
		Action:        "new",
		Target:        "opencode",
		WorkspacePath: "/tmp/fork-ws",
	})
	if err != nil {
		t.Fatalf("new parent: %v", err)
	}
	parentMeta, found, err := mgr.loadSessionMeta(parent.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("load parent: err=%v found=%v", err, found)
	}
	parentMeta.AgentSessionID = "parent-remote"
	if err := mgr.saveSessionMeta(parentMeta); err != nil {
		t.Fatalf("save parent: %v", err)
	}
	makeActive := false
	result, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "fork-ch",
		Action:        "fork",
		Target:        parentMeta.ID,
		MakeActive:    &makeActive,
		Ephemeral:     true,
		CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if result.Fork == nil || result.Fork.ChildLogicalSessionID == "" {
		t.Fatalf("expected fork result, got %+v", result)
	}
	if result.ActiveSessionID != parentMeta.ID {
		t.Fatalf("expected active parent preserved, got %q", result.ActiveSessionID)
	}
	if !result.Fork.ParentRestored || result.Fork.MakeActive {
		t.Fatalf("expected parent restored with inactive child, got %+v", result.Fork)
	}
	childMeta, found, err := mgr.loadSessionMeta(result.Fork.ChildLogicalSessionID)
	if err != nil || !found {
		t.Fatalf("load child: err=%v found=%v", err, found)
	}
	if childMeta.ParentSessionID != parentMeta.ID || childMeta.ParentRemoteID != "parent-remote" {
		t.Fatalf("expected parent linkage, got %+v", childMeta)
	}
	if !childMeta.Ephemeral || childMeta.CleanupPolicy != middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal {
		t.Fatalf("expected ephemeral cleanup policy, got %+v", childMeta)
	}
	if len(router.forked) != 1 || router.forked[0].RemoteSessionID != "parent-remote" {
		t.Fatalf("expected real fork call, got %+v", router.forked)
	}
	state, err := mgr.getChannelState("fork-ch")
	if err != nil {
		t.Fatalf("get channel state: %v", err)
	}
	if state.ActiveSessionID != parentMeta.ID {
		t.Fatalf("expected channel active parent, got %+v", state)
	}
}

func TestSessionManager_ForkMaterializesMissingParentRemote(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	parent, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "fork-materialize-ch",
		Action:        "new",
		Target:        "opencode",
		WorkspacePath: "/tmp/fork-materialize-ws",
		Ephemeral:     true,
		CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	if err != nil {
		t.Fatalf("new parent: %v", err)
	}
	parentMeta, found, err := mgr.loadSessionMeta(parent.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("load parent: err=%v found=%v", err, found)
	}
	if parentMeta.AgentSessionID != "" {
		t.Fatalf("new session should start without remote id, got %q", parentMeta.AgentSessionID)
	}
	makeActive := false
	result, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "fork-materialize-ch",
		Action:        "fork",
		Target:        parentMeta.ID,
		Input:         "return smoke artifact",
		MakeActive:    &makeActive,
		RestoreParent: true,
		Ephemeral:     true,
		CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if result.Error != nil || result.Unsupported {
		t.Fatalf("expected materialized fork, got %+v", result)
	}
	if len(router.materialized) != 1 {
		t.Fatalf("expected one parent materialization, got %+v", router.materialized)
	}
	if router.materialized[0].LogicalSessionID != parentMeta.ID || router.materialized[0].WorkspacePath != "/tmp/fork-materialize-ws" {
		t.Fatalf("unexpected materialize request: %+v", router.materialized[0])
	}
	if len(router.forked) != 1 || router.forked[0].RemoteSessionID != "materialized-parent-remote" {
		t.Fatalf("expected fork from materialized parent, got %+v", router.forked)
	}
	updatedParent, found, err := mgr.loadSessionMeta(parentMeta.ID)
	if err != nil || !found {
		t.Fatalf("load updated parent: err=%v found=%v", err, found)
	}
	if updatedParent.AgentSessionID != "materialized-parent-remote" || updatedParent.MirrorStatus != "mirrored" {
		t.Fatalf("expected parent remote persisted, got %+v", updatedParent)
	}
	if result.Fork == nil || result.Fork.ParentRemoteSessionID != "materialized-parent-remote" {
		t.Fatalf("expected materialized parent in fork result, got %+v", result.Fork)
	}
	if result.Fork.Artifact == nil || result.Fork.Artifact.Content != "Ok" {
		t.Fatalf("expected child artifact, got %+v", result.Fork)
	}
	if result.ActiveSessionID != parentMeta.ID || !result.Fork.ParentRestored {
		t.Fatalf("expected parent restored, got active=%q fork=%+v", result.ActiveSessionID, result.Fork)
	}
}

func TestSessionManager_ForkMissingParentRemoteReturnsTypedError(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{materialErr: fmt.Errorf("provider refused materialization")}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	parent, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "fork-missing-remote-ch",
		Action:        "new",
		Target:        "opencode",
		WorkspacePath: "/tmp/fork-missing-remote-ws",
	})
	if err != nil {
		t.Fatalf("new parent: %v", err)
	}
	result, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID: "fork-missing-remote-ch",
		Action:    "fork",
		Target:    parent.ActiveSessionID,
	})
	if err != nil {
		t.Fatalf("fork should return typed result, got err=%v", err)
	}
	if !result.Unsupported || result.Error == nil {
		t.Fatalf("expected typed unsupported result, got %+v", result)
	}
	if result.Error.Code != "remote_session_materialize_failed" || result.Error.Target != parent.ActiveSessionID {
		t.Fatalf("unexpected error: %+v", result.Error)
	}
	if result.Fork == nil || !result.Fork.Unsupported || !strings.Contains(result.Fork.Reason, "provider refused") {
		t.Fatalf("expected fork unsupported reason, got %+v", result.Fork)
	}
}

func TestSessionManager_ForkInputCanCleanupChildAndRestoreParent(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	parent, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "fork-run-ch",
		Action:        "new",
		Target:        "opencode",
		WorkspacePath: "/tmp/fork-run-ws",
	})
	if err != nil {
		t.Fatalf("new parent: %v", err)
	}
	parentMeta, found, err := mgr.loadSessionMeta(parent.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("load parent: err=%v found=%v", err, found)
	}
	parentMeta.AgentSessionID = "parent-run-remote"
	if err := mgr.saveSessionMeta(parentMeta); err != nil {
		t.Fatalf("save parent: %v", err)
	}
	result, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "fork-run-ch",
		Action:        "fork",
		Target:        parentMeta.ID,
		Input:         "summarize fork evidence",
		Ephemeral:     true,
		CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	if err != nil {
		t.Fatalf("fork with input: %v", err)
	}
	if result.Fork == nil || result.Fork.Artifact == nil || result.Fork.Artifact.Content != "Ok" {
		t.Fatalf("expected child artifact, got %+v", result.Fork)
	}
	if result.Fork.Cleanup == nil || !result.Fork.Cleanup.Clean || !result.Fork.Cleanup.StrongCleanup {
		t.Fatalf("expected strong cleanup proof, got %+v", result.Fork.Cleanup)
	}
	if result.ActiveSessionID != parentMeta.ID || !result.Fork.ParentRestored {
		t.Fatalf("expected parent restored, got active=%q fork=%+v", result.ActiveSessionID, result.Fork)
	}
	if router.lastSession != result.Fork.ChildLogicalSessionID {
		t.Fatalf("expected child turn routed to fork child, got %q want %q", router.lastSession, result.Fork.ChildLogicalSessionID)
	}
	raw, _ := storage.Get("session.meta." + result.Fork.ChildLogicalSessionID)
	if len(raw) != 0 {
		t.Fatalf("expected cleaned child meta removed, got %q", string(raw))
	}
	if len(router.reaped) != 0 {
		t.Fatalf("fork child cleanup must not reap shared parent client, got %+v", router.reaped)
	}
}

func TestSessionManager_ForkChildTurnFailureReturnsTypedCleanupProof(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{routeErr: fmt.Errorf("client context cancelled")}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	parent, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "fork-fail-ch",
		Action:        "new",
		Target:        "opencode",
		WorkspacePath: "/tmp/fork-fail-ws",
		Ephemeral:     true,
		CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	if err != nil {
		t.Fatalf("new parent: %v", err)
	}
	parentMeta, found, err := mgr.loadSessionMeta(parent.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("load parent: err=%v found=%v", err, found)
	}
	parentMeta.AgentSessionID = "parent-fail-remote"
	if err := mgr.saveSessionMeta(parentMeta); err != nil {
		t.Fatalf("save parent: %v", err)
	}
	makeActive := false
	result, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "fork-fail-ch",
		Action:        "fork",
		Target:        parentMeta.ID,
		Input:         "render diagnostic JSON",
		MakeActive:    &makeActive,
		RestoreParent: true,
		Ephemeral:     true,
		CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	if err != nil {
		t.Fatalf("fork failure must be typed, got err=%v", err)
	}
	if result.Error == nil || result.Error.Code != "fork_child_turn_failed" {
		t.Fatalf("expected typed child turn failure, got %+v", result)
	}
	if result.Fork == nil || result.Fork.Cleanup == nil || !result.Fork.Cleanup.Clean {
		t.Fatalf("expected cleanup proof on child failure, got %+v", result.Fork)
	}
	if result.ActiveSessionID != parentMeta.ID || !result.Fork.ParentRestored {
		t.Fatalf("expected parent restored after child failure, got active=%q fork=%+v", result.ActiveSessionID, result.Fork)
	}
	raw, _ := storage.Get("session.meta." + result.Fork.ChildLogicalSessionID)
	if len(raw) != 0 {
		t.Fatalf("expected failed child meta removed, got %q", string(raw))
	}
	if len(router.reaped) != 0 {
		t.Fatalf("child cleanup must not reap active parent client, got %+v", router.reaped)
	}
}

func TestSessionManager_EphemeralParentCleanupRetainsProcessWhenForkChildExists(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	parent := SessionMeta{
		ID:             "parent-active",
		AgentID:        "opencode",
		AgentSessionID: "parent-remote",
		ProtocolKind:   "acp",
		WorkspacePath:  "/tmp/active-parent-ws",
		Ephemeral:      true,
		CleanupPolicy:  middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	}
	child := SessionMeta{
		ID:              "child-active",
		AgentID:         "opencode",
		AgentSessionID:  "child-remote",
		ProtocolKind:    "acp",
		WorkspacePath:   "/tmp/active-parent-ws",
		ParentSessionID: parent.ID,
		ParentRemoteID:  parent.AgentSessionID,
		Ephemeral:       true,
		CleanupPolicy:   middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	}
	if err := mgr.saveSessionMeta(parent); err != nil {
		t.Fatalf("save parent: %v", err)
	}
	if err := mgr.saveSessionMeta(child); err != nil {
		t.Fatalf("save child: %v", err)
	}
	if err := mgr.updateChannelState("active-parent-ch", parent.ID); err != nil {
		t.Fatalf("channel state: %v", err)
	}
	if err := mgr.appendInactiveChannelSession("active-parent-ch", child.ID); err != nil {
		t.Fatalf("append child: %v", err)
	}

	result, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:        "active-parent-ch",
		Action:           "cleanup",
		Target:           parent.ID,
		CleanupPolicy:    middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
		ForceForgetLocal: true,
	})
	if err != nil {
		t.Fatalf("cleanup parent: %v", err)
	}
	if result.Cleanup == nil || !result.Cleanup.Clean || !result.Cleanup.ProcessRetained || !result.Cleanup.ProcessRetentionAllowed {
		t.Fatalf("expected retained clean parent cleanup, got %+v", result.Cleanup)
	}
	if len(router.reaped) != 0 {
		t.Fatalf("parent cleanup must not reap while fork child exists, got %+v", router.reaped)
	}
	if _, found, err := mgr.loadSessionMeta(child.ID); err != nil || !found {
		t.Fatalf("fork child should remain available: err=%v found=%v", err, found)
	}
}

func TestSessionManager_NewEphemeralSessionAcceptsWorkspacePathWithoutRegisteredWorkspace(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	mgr := NewManager(storage, &mockRouter{}, newTestWizard(storage), nil)

	result, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "eval-channel",
		Action:        "new",
		Target:        "opencode",
		WorkspacePath: "/tmp/matrix-eval-workspace",
		Ephemeral:     true,
		CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrForgetLocal,
	})
	if err != nil {
		t.Fatalf("HandleSessionActionTyped new: %v", err)
	}
	if result.Session == nil {
		t.Fatalf("expected session in result")
	}
	meta, found, err := mgr.loadSessionMeta(result.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	if meta.WorkspaceID != "" {
		t.Fatalf("expected no stable workspace id, got %q", meta.WorkspaceID)
	}
	if meta.WorkspacePath != "/tmp/matrix-eval-workspace" {
		t.Fatalf("expected workspace path, got %q", meta.WorkspacePath)
	}
	if !meta.Ephemeral {
		t.Fatalf("expected ephemeral session")
	}
	if meta.CleanupPolicy != middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal {
		t.Fatalf("expected normalized cleanup policy, got %q", meta.CleanupPolicy)
	}
}

func TestSessionManager_AttachRunContextUsesRunRemoteWhenMirrorLags(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	meta := SessionMeta{
		ID:             "logical-live",
		AgentID:        "opencode",
		AgentSessionID: "stale-mirror-remote",
		WorkspacePath:  "/tmp/live-ws",
		MirrorStatus:   "mirrored",
	}
	if err := mgr.saveSessionMeta(meta); err != nil {
		t.Fatalf("saveSessionMeta: %v", err)
	}

	result, err := mgr.AttachRunContext(context.Background(), middleware.RunContextAttachmentRequest{
		RunID:            "run-live",
		DeliveryID:       "ctx-live",
		AgentID:          "opencode",
		LogicalSessionID: "logical-live",
		RemoteSessionID:  "run-active-remote",
		Reason:           "noema_active_sidecar_suggestion",
		SidecarCapsules: []middleware.SidecarCapsule{
			{Provider: "noema", ID: "sug-1", Visibility: middleware.SidecarVisibilityLLMVisible, Content: "guidance"},
		},
	})
	if err != nil {
		t.Fatalf("AttachRunContext: %v", err)
	}
	if result.Unsupported || result.Status != "delivered" {
		t.Fatalf("expected delivered attach despite stale mirror, got %+v", result)
	}
	if router.lastSession != "logical-live" || router.lastRemote != "run-active-remote" {
		t.Fatalf("expected run-bound session routing, session=%q remote=%q", router.lastSession, router.lastRemote)
	}
	if !router.lastStrict {
		t.Fatalf("live attach must require the run-bound remote session")
	}
	updated, found, err := mgr.loadSessionMeta("logical-live")
	if err != nil || !found {
		t.Fatalf("load updated meta: err=%v found=%v", err, found)
	}
	if updated.AgentSessionID != "run-active-remote" {
		t.Fatalf("expected mirror repaired from live run remote, got %q", updated.AgentSessionID)
	}
}

func TestSessionManager_CleanupFallsBackToCancelAndForgetsLocalMirror(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{ID: "eval-ws", RootPath: "/tmp/eval-ws"}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{deleteErr: errors.New("ACP agent does not advertise session/delete")}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	newResult, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "eval-channel",
		Action:        "new",
		Target:        "opencode",
		WorkspaceID:   "eval-ws",
		Ephemeral:     true,
		CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrForgetLocal,
	})
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(newResult.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	meta.AgentSessionID = "ses_eval_remote"
	meta.ProtocolKind = string(middleware.ProtocolKindACP)
	if err := mgr.saveSessionMeta(meta); err != nil {
		t.Fatalf("saveSessionMeta: %v", err)
	}

	cleanup, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:        "eval-channel",
		Action:           "cleanup",
		Target:           meta.ID,
		CleanupPolicy:    middleware.SessionCleanupPolicyDeleteRemoteOrForgetLocal,
		ForceForgetLocal: true,
	})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if cleanup.Cleanup == nil {
		t.Fatalf("expected cleanup proof")
	}
	proof := cleanup.Cleanup
	if !proof.RemoteDeleteAttempted || proof.RemoteDeleted {
		t.Fatalf("unexpected remote delete proof: %+v", proof)
	}
	if !proof.RemoteDeleteUnsupported {
		t.Fatalf("expected delete unsupported proof: %+v", proof)
	}
	if !proof.RemoteCancelAttempted || !proof.RemoteCanceled {
		t.Fatalf("expected remote cancel fallback: %+v", proof)
	}
	if !proof.LocalForgotten {
		t.Fatalf("expected local forgotten proof: %+v", proof)
	}
	if !proof.ProcessReapAttempted || !proof.ProcessReaped {
		t.Fatalf("expected ephemeral process reap proof: %+v", proof)
	}
	if !proof.Clean {
		t.Fatalf("expected clean cleanup proof: %+v", proof)
	}
	if !proof.StrongCleanup || proof.CleanupStrength != sessioncleanup.StrengthStrong || proof.Error != "" {
		t.Fatalf("expected strong clean fallback without terminal error: %+v", proof)
	}
	if len(router.deleted) != 1 || router.deleted[0] != "ses_eval_remote" {
		t.Fatalf("expected delete attempt, got %+v", router.deleted)
	}
	if len(router.deletedWS) != 1 || router.deletedWS[0] != "ses_eval_remote|/tmp/eval-ws" {
		t.Fatalf("expected workspace-aware delete attempt, got %+v", router.deletedWS)
	}
	if len(router.canceled) != 1 || router.canceled[0] != "ses_eval_remote" {
		t.Fatalf("expected cancel fallback, got %+v", router.canceled)
	}
	if len(router.canceledWS) != 1 || router.canceledWS[0] != "ses_eval_remote|/tmp/eval-ws" {
		t.Fatalf("expected workspace-aware cancel fallback, got %+v", router.canceledWS)
	}
	if len(router.reaped) != 1 || router.reaped[0] != "opencode|/tmp/eval-ws" {
		t.Fatalf("expected ephemeral client reap, got %+v", router.reaped)
	}
	raw, _ := storage.Get("session.meta." + meta.ID)
	if len(raw) != 0 {
		t.Fatalf("expected local session meta removed, got %q", string(raw))
	}
	index, err := workspace.LoadSessionIndex(storage, "eval-ws")
	if err != nil {
		t.Fatalf("LoadSessionIndex: %v", err)
	}
	for _, id := range index {
		if id == meta.ID {
			t.Fatalf("expected workspace index cleanup, got %+v", index)
		}
	}
}

func TestSessionManager_CleanupPrefersCloseBeforeCancelWhenDeleteUnsupported(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	if err := workspace.SaveMeta(storage, workspace.Meta{ID: "eval-ws", RootPath: "/tmp/eval-ws"}); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	router := &mockRouter{
		deleteErr: errors.New("ACP agent does not advertise session/delete"),
		closeOK:   true,
	}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	newResult, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "eval-channel",
		Action:        "new",
		Target:        "codex",
		WorkspaceID:   "eval-ws",
		Ephemeral:     true,
		CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(newResult.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	meta.AgentSessionID = "ses_eval_remote_close"
	meta.ProtocolKind = string(middleware.ProtocolKindACP)
	if err := mgr.saveSessionMeta(meta); err != nil {
		t.Fatalf("saveSessionMeta: %v", err)
	}

	cleanup, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:        "eval-channel",
		Action:           "cleanup",
		Target:           meta.ID,
		CleanupPolicy:    middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
		ForceForgetLocal: true,
	})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if cleanup.Cleanup == nil {
		t.Fatalf("expected cleanup proof")
	}
	proof := cleanup.Cleanup
	if !proof.RemoteDeleteAttempted || !proof.RemoteDeleteUnsupported || proof.RemoteDeleted {
		t.Fatalf("unexpected remote delete proof: %+v", proof)
	}
	if !proof.RemoteCloseAttempted || !proof.RemoteClosed || proof.RemoteCloseUnsupported {
		t.Fatalf("expected successful remote close fallback: %+v", proof)
	}
	if proof.RemoteCancelAttempted || proof.RemoteCanceled {
		t.Fatalf("close success must avoid cancel fallback: %+v", proof)
	}
	if !proof.LocalForgotten || !proof.ProcessReapAttempted || !proof.ProcessReaped || !proof.Clean {
		t.Fatalf("expected clean local/process cleanup proof: %+v", proof)
	}
	if !proof.StrongCleanup || proof.CleanupStrength != sessioncleanup.StrengthStrong || proof.Error != "" {
		t.Fatalf("expected strong clean close fallback without terminal error: %+v", proof)
	}
	if len(router.closed) != 1 || router.closed[0] != "ses_eval_remote_close" {
		t.Fatalf("expected close fallback, got %+v", router.closed)
	}
	if len(router.closedWS) != 1 || router.closedWS[0] != "ses_eval_remote_close|/tmp/eval-ws" {
		t.Fatalf("expected workspace-aware close fallback, got %+v", router.closedWS)
	}
	if len(router.canceled) != 0 || len(router.canceledWS) != 0 {
		t.Fatalf("did not expect cancel fallback, canceled=%+v canceledWS=%+v", router.canceled, router.canceledWS)
	}
}

func TestSessionManager_CleanupFailureReturnsTypedProof(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{deleteErr: errors.New("provider refused cleanup")}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)

	newResult, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "eval-channel",
		Action:        "new",
		Target:        "opencode",
		Ephemeral:     true,
		CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemote,
	})
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(newResult.ActiveSessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	meta.AgentSessionID = "remote-parent"
	meta.ProtocolKind = string(middleware.ProtocolKindACP)
	if err := mgr.saveSessionMeta(meta); err != nil {
		t.Fatalf("saveSessionMeta: %v", err)
	}

	cleanup, err := mgr.HandleSessionActionTyped(context.Background(), middleware.SessionActionRequest{
		ChannelID:     "eval-channel",
		Action:        "cleanup",
		Target:        meta.ID,
		CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemote,
	})
	if err != nil {
		t.Fatalf("cleanup should return typed proof, got error: %v", err)
	}
	if cleanup.Error == nil || cleanup.Error.Code != "remote_delete" {
		t.Fatalf("expected typed remote_delete error, got %+v", cleanup)
	}
	if cleanup.Cleanup == nil {
		t.Fatalf("expected cleanup proof")
	}
	proof := cleanup.Cleanup
	if proof.Clean || proof.StrongCleanup || proof.LocalForgotten {
		t.Fatalf("expected failed cleanup proof without local forget, got %+v", proof)
	}
	if !proof.RemoteDeleteAttempted || proof.RemoteDeleted || proof.FailureCode != "remote_delete" {
		t.Fatalf("expected remote delete failure proof, got %+v", proof)
	}
	raw, _ := storage.Get("session.meta." + meta.ID)
	if len(raw) == 0 {
		t.Fatalf("delete_remote failure must preserve local mirror")
	}
}

func TestSessionManager_SessionDeleteDeletesRemoteOnlyTarget(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{
		remote: []middleware.RemoteSessionInfo{
			{RemoteSessionID: "a2a-remote-1", DisplayID: "task-1", ProtocolKind: middleware.ProtocolKindA2A},
		},
	}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	if _, err := mgr.GetOrCreateSession("telegram_4", "planner"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	got, err := mgr.Route(context.Background(), "telegram_4", "planner", "/session delete task-1", nil)
	if err != nil {
		t.Fatalf("Route delete remote failed: %v", err)
	}
	if !contains(got, "Deleted remote a2a session: task-1") {
		t.Fatalf("expected remote delete response, got %q", got)
	}
	if len(router.deleted) != 1 || router.deleted[0] != "a2a-remote-1" {
		t.Fatalf("expected remote delete for task-1, got %+v", router.deleted)
	}
}

func TestSessionManager_SessionCancelCancelsMirrorAndUpdatesStatus(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	sessionID, err := mgr.GetOrCreateSession("telegram_5", "codex")
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(sessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	meta.AgentSessionID = "acp-remote-cancel"
	if err := mgr.saveSessionMeta(meta); err != nil {
		t.Fatalf("saveSessionMeta: %v", err)
	}

	got, err := mgr.Route(context.Background(), "telegram_5", "codex", "/session cancel", nil)
	if err != nil {
		t.Fatalf("Route cancel failed: %v", err)
	}
	if !contains(got, "Canceled session:") {
		t.Fatalf("expected cancel response, got %q", got)
	}
	if len(router.canceled) != 1 || router.canceled[0] != "acp-remote-cancel" {
		t.Fatalf("expected remote cancel call, got %+v", router.canceled)
	}
	meta, found, err = mgr.loadSessionMeta(sessionID)
	if err != nil || !found {
		t.Fatalf("reload session meta: err=%v found=%v", err, found)
	}
	if meta.RemoteStatus != "canceled" {
		t.Fatalf("expected remote status canceled, got %q", meta.RemoteStatus)
	}
}

func TestSessionManager_SessionCancelCancelsRemoteOnlyTarget(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{
		remote: []middleware.RemoteSessionInfo{
			{RemoteSessionID: "a2a-remote-cancel-1", DisplayID: "task-cancel-1", ProtocolKind: middleware.ProtocolKindA2A},
		},
	}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	if _, err := mgr.GetOrCreateSession("telegram_6", "planner"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	got, err := mgr.Route(context.Background(), "telegram_6", "planner", "/session cancel task-cancel-1", nil)
	if err != nil {
		t.Fatalf("Route cancel remote failed: %v", err)
	}
	if !contains(got, "Canceled remote a2a session: task-cancel-1") {
		t.Fatalf("expected remote cancel response, got %q", got)
	}
	if len(router.canceled) != 1 || router.canceled[0] != "a2a-remote-cancel-1" {
		t.Fatalf("expected remote cancel for task-cancel-1, got %+v", router.canceled)
	}
}

func TestSessionManager_CancelAliasCancelsActiveSession(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	sessionID, err := mgr.GetOrCreateSession("telegram_7", "codex")
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}
	meta, found, err := mgr.loadSessionMeta(sessionID)
	if err != nil || !found {
		t.Fatalf("loadSessionMeta: err=%v found=%v", err, found)
	}
	meta.AgentSessionID = "acp-remote-alias"
	if err := mgr.saveSessionMeta(meta); err != nil {
		t.Fatalf("saveSessionMeta: %v", err)
	}

	got, err := mgr.Route(context.Background(), "telegram_7", "codex", "/cancel", nil)
	if err != nil {
		t.Fatalf("Route cancel alias failed: %v", err)
	}
	if !contains(got, "Canceled session:") {
		t.Fatalf("expected cancel alias response, got %q", got)
	}
	if len(router.canceled) != 1 || router.canceled[0] != "acp-remote-alias" {
		t.Fatalf("expected remote cancel call, got %+v", router.canceled)
	}
}

func TestSessionManager_StopAliasCancelsRemoteTarget(t *testing.T) {
	storage := &mockStorage{data: make(map[string][]byte)}
	if err := storage.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("Failed to set configured flag: %v", err)
	}
	router := &mockRouter{
		remote: []middleware.RemoteSessionInfo{
			{RemoteSessionID: "a2a-remote-stop-1", DisplayID: "task-stop-1", ProtocolKind: middleware.ProtocolKindA2A},
		},
	}
	mgr := NewManager(storage, router, newTestWizard(storage), nil)
	if _, err := mgr.GetOrCreateSession("telegram_8", "planner"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	got, err := mgr.Route(context.Background(), "telegram_8", "planner", "/stop task-stop-1", nil)
	if err != nil {
		t.Fatalf("Route stop alias failed: %v", err)
	}
	if !contains(got, "Canceled remote a2a session: task-stop-1") {
		t.Fatalf("expected stop alias response, got %q", got)
	}
	if len(router.canceled) != 1 || router.canceled[0] != "a2a-remote-stop-1" {
		t.Fatalf("expected remote cancel for stop alias, got %+v", router.canceled)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
