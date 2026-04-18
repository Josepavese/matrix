package runapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jose/matrix-v2/internal/logic/memstore"
	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/middleware"
)

type runTestRouter struct {
	lastConversation middleware.ConversationRequest
	sessionActions   []middleware.SessionActionRequest
	routeErr         error
}

func (r *runTestRouter) Route(_ context.Context, channelID string, agentID string, input string, _ middleware.ThoughtNotifier) (string, error) {
	r.lastConversation = middleware.ConversationRequest{ChannelID: channelID, AgentID: agentID, Input: input}
	if r.routeErr != nil {
		return "", r.routeErr
	}
	return "ok", nil
}

func (r *runTestRouter) RouteConversation(_ context.Context, req middleware.ConversationRequest) (string, error) {
	r.lastConversation = req
	if r.routeErr != nil {
		return "", r.routeErr
	}
	return "ok", nil
}

func (r *runTestRouter) HandleSessionAction(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

func (r *runTestRouter) HandleSessionActionTyped(_ context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	r.sessionActions = append(r.sessionActions, req)
	switch req.Action {
	case "new":
		return middleware.SessionActionResult{
			Action:          "new",
			ActiveSessionID: "logical-eval",
			Session: &middleware.SessionEntry{
				LogicalSessionID: "logical-eval",
				AgentID:          req.Target,
				WorkspacePath:    req.WorkspacePath,
				Ephemeral:        req.Ephemeral,
				CleanupPolicy:    req.CleanupPolicy,
				Active:           true,
			},
		}, nil
	case "status":
		return middleware.SessionActionResult{
			Action:          "status",
			ActiveSessionID: "logical-eval",
			Session: &middleware.SessionEntry{
				LogicalSessionID: "logical-eval",
				RemoteSessionID:  "remote-eval",
				AgentID:          "opencode",
				ProtocolKind:     "acp",
				WorkspacePath:    "/tmp/eval-ws",
				Status:           "active",
				Active:           true,
			},
		}, nil
	case "cleanup":
		return middleware.SessionActionResult{
			Action: "cleanup",
			Cleanup: &middleware.SessionCleanupResult{
				LogicalSessionID:        req.Target,
				RemoteSessionID:         "remote-eval",
				AgentID:                 "opencode",
				ProtocolKind:            "acp",
				CleanupPolicy:           req.CleanupPolicy,
				Clean:                   true,
				RemoteDeleteAttempted:   true,
				RemoteDeleteUnsupported: true,
				RemoteCancelAttempted:   true,
				RemoteCanceled:          true,
				ProcessReapAttempted:    true,
				ProcessReaped:           true,
				LocalForgotten:          true,
			},
		}, nil
	default:
		return middleware.SessionActionResult{Action: req.Action}, nil
	}
}

func (r *runTestRouter) HandleWorkspaceAction(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (r *runTestRouter) HandleWorkspaceActionTyped(context.Context, middleware.WorkspaceActionRequest) (middleware.WorkspaceActionResult, error) {
	return middleware.WorkspaceActionResult{}, nil
}

func (r *runTestRouter) HandleWorkspaceRead(context.Context, string, string, string, int) (string, error) {
	return "", nil
}

func (r *runTestRouter) HandleWorkspaceReadTyped(context.Context, middleware.WorkspaceReadRequest) (middleware.WorkspaceReadResult, error) {
	return middleware.WorkspaceReadResult{}, nil
}

func (r *runTestRouter) HandleIntent(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (r *runTestRouter) HandleIntentTyped(context.Context, middleware.IntentActionRequest) (middleware.IntentActionResult, error) {
	return middleware.IntentActionResult{}, nil
}

func TestHandleRuns_NewEphemeralDeleteAfterRunCreatesCleansAndTraces(t *testing.T) {
	router := &runTestRouter{}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "noema-eval-channel-random",
		"agent_id":       "opencode",
		"input":          "run eval",
		"workspace_path": "/tmp/eval-ws",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": middleware.SessionCleanupPolicyDeleteRemoteOrForgetLocal,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastConversation.WorkspacePath != "/tmp/eval-ws" {
		t.Fatalf("expected workspace path routed, got %q", router.lastConversation.WorkspacePath)
	}
	if len(router.sessionActions) < 3 {
		t.Fatalf("expected new/status/cleanup actions, got %+v", router.sessionActions)
	}
	if router.sessionActions[0].Action != "status" {
		t.Fatalf("expected initial status snapshot first, got %+v", router.sessionActions[0])
	}
	if router.sessionActions[1].Action != "new" || !router.sessionActions[1].Ephemeral {
		t.Fatalf("expected ephemeral new action, got %+v", router.sessionActions[1])
	}
	if router.sessionActions[len(router.sessionActions)-1].Action != "cleanup" || !router.sessionActions[len(router.sessionActions)-1].ForceForgetLocal {
		t.Fatalf("expected force cleanup action, got %+v", router.sessionActions[len(router.sessionActions)-1])
	}

	var resp runResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Cleanup == nil || !resp.Cleanup.RemoteCanceled || !resp.Cleanup.LocalForgotten {
		t.Fatalf("expected cleanup proof in response, got %+v", resp.Cleanup)
	}
	events, err := server.Store().LoadEvents(resp.RunID, 100)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if !hasEventKind(events, "session.policy.applied") || !hasEventKind(events, "session.cleanup") {
		t.Fatalf("expected policy and cleanup events, got %+v", events)
	}
}

func TestHandleRuns_NewEphemeralDeleteAfterRunCleansWhenRouteFails(t *testing.T) {
	router := &runTestRouter{routeErr: errors.New("route failed")}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "eval-channel-route-fail",
		"agent_id":       "opencode",
		"input":          "run eval",
		"workspace_path": "/tmp/eval-ws",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if !hasSessionAction(router.sessionActions, "cleanup") {
		t.Fatalf("expected cleanup action after route failure, got %+v", router.sessionActions)
	}
	var resp runErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v; body=%s", err, w.Body.String())
	}
	if resp.Cleanup == nil || !resp.Cleanup.Clean || !resp.Cleanup.LocalForgotten {
		t.Fatalf("expected cleanup proof in error response, got %+v", resp.Cleanup)
	}
}

func TestHandleRuns_CleanupPolicyAloneDoesNotCleanupActiveSession(t *testing.T) {
	router := &runTestRouter{}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "regular-channel",
		"agent_id":       "opencode",
		"input":          "regular run",
		"cleanup_policy": middleware.SessionCleanupPolicyForgetLocal,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if hasSessionAction(router.sessionActions, "cleanup") {
		t.Fatalf("cleanup_policy without ephemeral session_policy must not cleanup active session: %+v", router.sessionActions)
	}
}

func hasEventKind(events []runtrace.Event, kind string) bool {
	for _, event := range events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func hasSessionAction(actions []middleware.SessionActionRequest, action string) bool {
	for _, req := range actions {
		if req.Action == action {
			return true
		}
	}
	return false
}
