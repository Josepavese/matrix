package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jose/matrix-v2/internal/logic/orchestration"
	"github.com/jose/matrix-v2/internal/middleware"
)

type mockSessionRouter struct {
	lastChannelID        string
	lastInput            string
	lastAgentID          string
	lastWorkspaceID      string
	lastWorkspacePath    string
	lastAction           string
	lastTarget           string
	lastIntentAgentID    string
	lastIntentNote       string
	response             string
	err                  error
	authResponse         string
	authErr              error
	typedResult          middleware.SessionActionResult
	workspaceTypedResult middleware.WorkspaceActionResult
	workspaceReadResult  middleware.WorkspaceReadResult
	intentTypedResult    middleware.IntentActionResult
}

func (m *mockSessionRouter) Route(_ context.Context, channelID, agentID, input string, _ middleware.ThoughtNotifier) (string, error) {
	m.lastChannelID = channelID
	m.lastAgentID = agentID
	m.lastInput = input
	return m.response, m.err
}

func (m *mockSessionRouter) RouteConversation(_ context.Context, req middleware.ConversationRequest) (string, error) {
	m.lastChannelID = req.ChannelID
	m.lastAgentID = req.AgentID
	m.lastInput = req.Input
	m.lastWorkspaceID = req.WorkspaceID
	m.lastWorkspacePath = req.WorkspacePath
	return m.response, m.err
}

func (m *mockSessionRouter) HandleSessionAction(_ context.Context, channelID, action, target string) (string, error) {
	m.lastChannelID = channelID
	m.lastAction = action
	m.lastTarget = target
	return m.response, m.err
}

func (m *mockSessionRouter) HandleSessionActionTyped(_ context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	m.lastChannelID = req.ChannelID
	m.lastAction = req.Action
	m.lastTarget = req.Target
	m.lastWorkspaceID = req.WorkspaceID
	if m.typedResult.Action == "" {
		m.typedResult = middleware.SessionActionResult{Action: req.Action, Message: m.response}
	}
	return m.typedResult, m.err
}

func (m *mockSessionRouter) HandleAuthCallback(channelID, _, _ string) (string, error) {
	m.lastChannelID = channelID
	return m.authResponse, m.authErr
}

func (m *mockSessionRouter) HandleWorkspaceAction(_ context.Context, channelID, action, target string) (string, error) {
	m.lastChannelID = channelID
	m.lastAction = action
	m.lastTarget = target
	return m.response, m.err
}

func (m *mockSessionRouter) HandleWorkspaceActionTyped(_ context.Context, req middleware.WorkspaceActionRequest) (middleware.WorkspaceActionResult, error) {
	m.lastChannelID = req.ChannelID
	m.lastAction = req.Action
	m.lastTarget = req.Target
	if m.workspaceTypedResult.Action == "" {
		m.workspaceTypedResult = middleware.WorkspaceActionResult{Action: req.Action, Message: m.response}
	}
	return m.workspaceTypedResult, m.err
}

func (m *mockSessionRouter) HandleWorkspaceRead(_ context.Context, channelID, action, workspaceID string, _ int) (string, error) {
	m.lastChannelID = channelID
	m.lastAction = action
	m.lastWorkspaceID = workspaceID
	return m.response, m.err
}

func (m *mockSessionRouter) HandleWorkspaceReadTyped(_ context.Context, req middleware.WorkspaceReadRequest) (middleware.WorkspaceReadResult, error) {
	m.lastChannelID = req.ChannelID
	m.lastAction = req.Action
	m.lastWorkspaceID = req.WorkspaceID
	if m.workspaceReadResult.Action == "" {
		m.workspaceReadResult = middleware.WorkspaceReadResult{Action: req.Action, Message: m.response}
	}
	return m.workspaceReadResult, m.err
}

func (m *mockSessionRouter) HandleIntent(_ context.Context, channelID, intent, target string) (string, error) {
	m.lastChannelID = channelID
	m.lastAction = intent
	m.lastTarget = target
	return m.response, m.err
}

func (m *mockSessionRouter) HandleIntentTyped(_ context.Context, req middleware.IntentActionRequest) (middleware.IntentActionResult, error) {
	m.lastChannelID = req.ChannelID
	m.lastAction = req.Intent
	m.lastTarget = req.Target
	m.lastWorkspaceID = req.WorkspaceID
	m.lastIntentAgentID = req.AgentID
	m.lastIntentNote = req.Note
	if m.intentTypedResult.Intent == "" {
		m.intentTypedResult = middleware.IntentActionResult{Intent: req.Intent, Message: m.response}
	}
	return m.intentTypedResult, m.err
}

func setupServer(router *mockSessionRouter, apiKey string, defaultAgent string) (*Server, *http.ServeMux) {
	s := NewServer(router)
	if apiKey != "" {
		s.WithAPIKey(apiKey)
	}
	if defaultAgent != "" {
		s.WithDefaultAgent(defaultAgent)
	}
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	return s, mux
}

func TestHandleRuns_MethodNotAllowed(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "", "")
	req := httptest.NewRequest(http.MethodGet, RunPathV1, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleRuns_InvalidJSON(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "", "")
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRuns_MissingFields(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "", "")
	body, _ := json.Marshal(map[string]string{"channel_id": "ch1"})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing input, got %d", w.Code)
	}
}

func TestHandleRuns_Unauthorized(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "secret-key", "")

	// No API key
	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "input": "hi"})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	// Wrong API key
	req = httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	req.Header.Set("X-Matrix-Key", "wrong")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong key, got %d", w.Code)
	}
}

func TestHandleRuns_Success(t *testing.T) {
	router := &mockSessionRouter{response: "Hello from agent"}
	_, mux := setupServer(router, "secret-key", "")

	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "input": "hello", "agent_id": "gemini", "workspace_id": "billing-api", "workspace_path": "/tmp/billing-api"})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	req.Header.Set("X-Matrix-Key", "secret-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastChannelID != "ch1" {
		t.Errorf("expected channelID ch1, got %s", router.lastChannelID)
	}
	if router.lastAgentID != "gemini" {
		t.Errorf("expected agentID gemini, got %s", router.lastAgentID)
	}
	if router.lastWorkspaceID != "billing-api" {
		t.Errorf("expected workspaceID billing-api, got %s", router.lastWorkspaceID)
	}
	if router.lastWorkspacePath != "/tmp/billing-api" {
		t.Errorf("expected workspacePath /tmp/billing-api, got %s", router.lastWorkspacePath)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response parse error: %v", err)
	}
	if resp["output"] != "Hello from agent" {
		t.Errorf("unexpected output: %s", resp["output"])
	}
}

func TestHandleRuns_DefaultAgent(t *testing.T) {
	router := &mockSessionRouter{response: "ok"}
	_, mux := setupServer(router, "", "gemini")

	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "input": "test"})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if router.lastAgentID != "gemini" {
		t.Errorf("expected default agent 'gemini', got %s", router.lastAgentID)
	}
}

func TestHandleOpenRouterCallback_MissingParams(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "", "")

	req := httptest.NewRequest(http.MethodGet, OpenRouterCallbackV1, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleOpenRouterCallback_MethodNotAllowed(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "", "")
	req := httptest.NewRequest(http.MethodPost, OpenRouterCallbackV1+"?code=x&state=y", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleOpenRouterCallback_Success(t *testing.T) {
	router := &mockSessionRouter{authResponse: "Auth OK"}
	_, mux := setupServer(router, "", "")

	req := httptest.NewRequest(http.MethodGet, OpenRouterCallbackV1+"?code=abc123&state=ch1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Auth OK")) {
		t.Errorf("response should contain auth result: %s", w.Body.String())
	}
}

func TestHandleSessionActions_MethodNotAllowed(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "", "")
	req := httptest.NewRequest(http.MethodGet, SessionActionPathV1, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleSessionActions_InvalidJSON(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "", "")
	req := httptest.NewRequest(http.MethodPost, SessionActionPathV1, bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSessionActions_MissingFields(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "", "")
	body, _ := json.Marshal(map[string]string{"channel_id": "ch1"})
	req := httptest.NewRequest(http.MethodPost, SessionActionPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSessionActions_ForwardsWorkspaceID(t *testing.T) {
	router := &mockSessionRouter{typedResult: middleware.SessionActionResult{Action: "status"}}
	_, mux := setupServer(router, "", "")
	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "action": "status", "workspace_id": "billing-api"})
	req := httptest.NewRequest(http.MethodPost, SessionActionPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if router.lastWorkspaceID != "billing-api" {
		t.Fatalf("expected workspace id to be forwarded, got %q", router.lastWorkspaceID)
	}
}

func TestHandleWorkspaceActions_Success(t *testing.T) {
	router := &mockSessionRouter{workspaceTypedResult: middleware.WorkspaceActionResult{
		Action:  "list",
		Message: "Configured workspaces",
		Workspaces: []middleware.WorkspaceEntry{
			{ID: "billing-api", RootPath: "/tmp/billing-api", Active: true},
		},
	}}
	_, mux := setupServer(router, "", "")
	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "action": "list"})
	req := httptest.NewRequest(http.MethodPost, WorkspaceActionPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastAction != "list" {
		t.Fatalf("expected workspace action list, got %q", router.lastAction)
	}
	var resp middleware.WorkspaceActionResult
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response parse error: %v", err)
	}
	if len(resp.Workspaces) != 1 || resp.Workspaces[0].ID != "billing-api" {
		t.Fatalf("unexpected workspace response: %+v", resp)
	}
}

func TestHandleIntents_Success(t *testing.T) {
	router := &mockSessionRouter{intentTypedResult: middleware.IntentActionResult{
		Intent:    "review",
		Message:   "Review mode enabled",
		Workspace: &middleware.WorkspaceEntry{ID: "billing-api", Active: true},
		Session:   &middleware.SessionEntry{LogicalSessionID: "sess-1", AgentID: "claude", WorkspaceID: "billing-api", Active: true},
	}}
	_, mux := setupServer(router, "", "")
	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "intent": "review", "target": "billing-api"})
	req := httptest.NewRequest(http.MethodPost, IntentActionPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastAction != "review" || router.lastTarget != "billing-api" {
		t.Fatalf("unexpected routed intent: action=%q target=%q", router.lastAction, router.lastTarget)
	}
}

func TestHandleIntents_HandoffPayload(t *testing.T) {
	router := &mockSessionRouter{intentTypedResult: middleware.IntentActionResult{
		Intent:  "handoff",
		Message: "Handed off workspace billing-api to claude",
	}}
	_, mux := setupServer(router, "", "")
	body, _ := json.Marshal(map[string]string{
		"channel_id":   "ch1",
		"intent":       "handoff",
		"workspace_id": "billing-api",
		"agent_id":     "claude",
		"note":         "Please review the current patch.",
	})
	req := httptest.NewRequest(http.MethodPost, IntentActionPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastAction != "handoff" || router.lastWorkspaceID != "billing-api" || router.lastIntentAgentID != "claude" {
		t.Fatalf("unexpected handoff routing: action=%q workspace=%q agent=%q", router.lastAction, router.lastWorkspaceID, router.lastIntentAgentID)
	}
	if router.lastIntentNote != "Please review the current patch." {
		t.Fatalf("unexpected handoff note: %q", router.lastIntentNote)
	}
}

func TestHandleWorkspaceState_Success(t *testing.T) {
	router := &mockSessionRouter{workspaceReadResult: middleware.WorkspaceReadResult{
		Action: "state",
		State: &middleware.WorkspaceStateEntry{
			WorkspaceID:            "billing-api",
			ActiveLogicalSessionID: "sess-1",
			ActiveAgentID:          "claude",
			ActiveMode:             "review",
		},
	}}
	_, mux := setupServer(router, "", "")
	req := httptest.NewRequest(http.MethodGet, WorkspaceStatePathV1+"?workspace_id=billing-api", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastAction != "state" || router.lastWorkspaceID != "billing-api" {
		t.Fatalf("unexpected workspace state routing: action=%q workspace=%q", router.lastAction, router.lastWorkspaceID)
	}
}

func TestHandleWorkspaceTimeline_Success(t *testing.T) {
	router := &mockSessionRouter{workspaceReadResult: middleware.WorkspaceReadResult{
		Action: "timeline",
		Timeline: []middleware.WorkspaceTimelineEvent{
			{Type: "handoff.created", WorkspaceID: "billing-api"},
		},
	}}
	_, mux := setupServer(router, "", "")
	req := httptest.NewRequest(http.MethodGet, WorkspaceTimelinePathV1+"?channel_id=telegram.user123", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastAction != "timeline" || router.lastChannelID != "telegram.user123" {
		t.Fatalf("unexpected workspace timeline routing: action=%q channel=%q", router.lastAction, router.lastChannelID)
	}
}

func TestHandleWorkspaceDecisions_Success(t *testing.T) {
	router := &mockSessionRouter{workspaceReadResult: middleware.WorkspaceReadResult{
		Action:    "decisions",
		Decisions: []middleware.WorkspaceDecisionTrace{{Kind: "resume-workspace-session", Explanation: "Resumed an existing workspace session."}},
	}}
	_, mux := setupServer(router, "", "")
	req := httptest.NewRequest(http.MethodGet, WorkspaceDecisionsPathV1+"?workspace_id=billing-api&limit=5", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastAction != "decisions" || router.lastWorkspaceID != "billing-api" {
		t.Fatalf("unexpected workspace decisions routing: action=%q workspace=%q", router.lastAction, router.lastWorkspaceID)
	}
}

func TestHandleWorkspaceMemory_Success(t *testing.T) {
	router := &mockSessionRouter{workspaceReadResult: middleware.WorkspaceReadResult{
		Action: "memory",
		Memory: []middleware.WorkspaceMemoryTurn{{Role: "user", WorkspaceID: "billing-api"}},
	}}
	_, mux := setupServer(router, "", "")
	req := httptest.NewRequest(http.MethodGet, WorkspaceMemoryPathV1+"?workspace_id=billing-api&limit=5", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastAction != "memory" || router.lastWorkspaceID != "billing-api" {
		t.Fatalf("unexpected workspace memory routing: action=%q workspace=%q", router.lastAction, router.lastWorkspaceID)
	}
}

func TestHandleWorkspaceSnapshots_Success(t *testing.T) {
	router := &mockSessionRouter{workspaceReadResult: middleware.WorkspaceReadResult{
		Action:    "snapshots",
		Snapshots: []middleware.WorkspaceSnapshotEntry{{WorkspaceID: "billing-api", Title: "snapshot"}},
	}}
	_, mux := setupServer(router, "", "")
	req := httptest.NewRequest(http.MethodGet, WorkspaceSnapshotsPathV1+"?channel_id=telegram.user123", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastAction != "snapshots" || router.lastChannelID != "telegram.user123" {
		t.Fatalf("unexpected workspace snapshots routing: action=%q channel=%q", router.lastAction, router.lastChannelID)
	}
}

func TestHandleModes_Success(t *testing.T) {
	router := &mockSessionRouter{intentTypedResult: middleware.IntentActionResult{
		Intent:  "explain",
		Message: "Explain mode enabled",
	}}
	_, mux := setupServer(router, "", "")
	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "mode": "explain", "target": "billing-api"})
	req := httptest.NewRequest(http.MethodPost, ModeActionPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastAction != "explain" || router.lastTarget != "billing-api" {
		t.Fatalf("unexpected routed mode action: action=%q target=%q", router.lastAction, router.lastTarget)
	}
}

func TestHandleOrchestrationCapabilities_Success(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "", "")
	req := httptest.NewRequest(http.MethodGet, OrchestrationProfileV1, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var profile orchestration.Profile
	if err := json.Unmarshal(w.Body.Bytes(), &profile); err != nil {
		t.Fatalf("response parse error: %v", err)
	}
	if profile.Category != "local-first Agent Session Fabric" {
		t.Fatalf("unexpected category: %q", profile.Category)
	}
	if len(profile.Capabilities) == 0 || len(profile.Surfaces) == 0 {
		t.Fatalf("expected orchestration capabilities and surfaces, got %+v", profile)
	}
	foundRuns := false
	for _, surface := range profile.Surfaces {
		if surface.ID == "http:/v1/runs" {
			foundRuns = true
			break
		}
	}
	if !foundRuns {
		t.Fatalf("expected http:/v1/runs surface in orchestration profile")
	}
}

func TestHandleSessionActions_Unauthorized(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "secret-key", "")
	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "action": "status"})

	req := httptest.NewRequest(http.MethodPost, SessionActionPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without key, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, SessionActionPathV1, bytes.NewReader(body))
	req.Header.Set("X-Matrix-Key", "wrong")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong key, got %d", w.Code)
	}
}

func TestHandleSessionActions_Success(t *testing.T) {
	router := &mockSessionRouter{typedResult: middleware.SessionActionResult{Action: "cancel", Message: "Canceled session: abc"}}
	_, mux := setupServer(router, "secret-key", "")

	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "action": "cancel", "target": "task-1"})
	req := httptest.NewRequest(http.MethodPost, SessionActionPathV1, bytes.NewReader(body))
	req.Header.Set("X-Matrix-Key", "secret-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastChannelID != "ch1" || router.lastAction != "cancel" || router.lastTarget != "task-1" {
		t.Fatalf("unexpected action routing: channel=%s action=%s target=%s", router.lastChannelID, router.lastAction, router.lastTarget)
	}
	var resp middleware.SessionActionResult
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response parse error: %v", err)
	}
	if resp.Action != "cancel" || resp.Message != "Canceled session: abc" {
		t.Fatalf("unexpected typed response: %+v", resp)
	}
}

func TestHandleSessionActions_ListSuccess(t *testing.T) {
	router := &mockSessionRouter{typedResult: middleware.SessionActionResult{Action: "list", Sessions: []middleware.SessionEntry{{LogicalSessionID: "s1", AgentID: "gemini", Active: true}}}}
	_, mux := setupServer(router, "", "")

	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "action": "list"})
	req := httptest.NewRequest(http.MethodPost, SessionActionPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastAction != "list" {
		t.Fatalf("expected action=list, got %s", router.lastAction)
	}
	var resp middleware.SessionActionResult
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response parse error: %v", err)
	}
	if resp.Action != "list" || len(resp.Sessions) != 1 {
		t.Fatalf("unexpected typed list response: %+v", resp)
	}
}

func TestHandleSessionActions_StatusSuccess(t *testing.T) {
	router := &mockSessionRouter{typedResult: middleware.SessionActionResult{
		Action:          "status",
		ActiveSessionID: "s1",
		Session:         &middleware.SessionEntry{LogicalSessionID: "s1", AgentID: "gemini", Active: true},
	}}
	_, mux := setupServer(router, "", "")

	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "action": "status"})
	req := httptest.NewRequest(http.MethodPost, SessionActionPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastAction != "status" {
		t.Fatalf("expected action=status, got %s", router.lastAction)
	}
	var resp middleware.SessionActionResult
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response parse error: %v", err)
	}
	if resp.Action != "status" || resp.Session == nil || resp.Session.LogicalSessionID != "s1" {
		t.Fatalf("unexpected typed status response: %+v", resp)
	}
}

func TestHandleSessionActions_NewAndNamePassThrough(t *testing.T) {
	router := &mockSessionRouter{typedResult: middleware.SessionActionResult{
		Action:          "new",
		ActiveSessionID: "s2",
		Session:         &middleware.SessionEntry{LogicalSessionID: "s2", AgentID: "claude", Active: true},
	}}
	_, mux := setupServer(router, "", "")

	newBody, _ := json.Marshal(map[string]string{"channel_id": "ch1", "action": "new", "target": "claude"})
	newReq := httptest.NewRequest(http.MethodPost, SessionActionPathV1, bytes.NewReader(newBody))
	newW := httptest.NewRecorder()
	mux.ServeHTTP(newW, newReq)

	if newW.Code != http.StatusCreated {
		t.Fatalf("expected 201 for new, got %d: %s", newW.Code, newW.Body.String())
	}
	if router.lastAction != "new" || router.lastTarget != "claude" {
		t.Fatalf("unexpected new routing: action=%s target=%s", router.lastAction, router.lastTarget)
	}

	router.typedResult = middleware.SessionActionResult{
		Action:          "name",
		Message:         "Session alias set: bugfix",
		ActiveSessionID: "s2",
		Session:         &middleware.SessionEntry{LogicalSessionID: "s2", AgentID: "claude", Alias: "bugfix", Active: true},
	}
	nameBody, _ := json.Marshal(map[string]string{"channel_id": "ch1", "action": "name", "target": "bugfix"})
	nameReq := httptest.NewRequest(http.MethodPost, SessionActionPathV1, bytes.NewReader(nameBody))
	nameW := httptest.NewRecorder()
	mux.ServeHTTP(nameW, nameReq)

	if nameW.Code != http.StatusCreated {
		t.Fatalf("expected 201 for name, got %d: %s", nameW.Code, nameW.Body.String())
	}
	if router.lastAction != "name" || router.lastTarget != "bugfix" {
		t.Fatalf("unexpected name routing: action=%s target=%s", router.lastAction, router.lastTarget)
	}
	var resp middleware.SessionActionResult
	if err := json.Unmarshal(nameW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response parse error: %v", err)
	}
	if resp.Action != "name" || resp.Session == nil || resp.Session.Alias != "bugfix" {
		t.Fatalf("unexpected typed name response: %+v", resp)
	}
}
