package runapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
	runresponse "github.com/Josepavese/matrix/internal/providers/runapi/response"
)

type runTestRouter struct {
	lastConversation middleware.ConversationRequest
	sessionActions   []middleware.SessionActionRequest
	newSessionID     string
	statusQueue      []middleware.SessionEntry
	listQueue        [][]middleware.SessionEntry
	cleanupByTarget  map[string]middleware.SessionCleanupResult
	attachMu         sync.Mutex
	attachRequests   []middleware.RunContextAttachmentRequest
	routeThoughts    []middleware.ThoughtUpdate
	routeErr         error
	routeWaitCancel  bool
	routeStarted     chan struct{}
	routeStartOnce   sync.Once
	cleanupCtxErr    chan error
	reconcile        *middleware.AgentClientReconcileResult
	reconcileErr     error
}

type safeLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (r *runTestRouter) Route(_ context.Context, channelID string, agentID string, input string, _ middleware.ThoughtNotifier) (string, error) {
	r.lastConversation = middleware.ConversationRequest{ChannelID: channelID, AgentID: agentID, Input: input}
	if r.routeErr != nil {
		return "", r.routeErr
	}
	return "ok", nil
}

func (r *runTestRouter) RouteConversation(ctx context.Context, req middleware.ConversationRequest) (string, error) {
	r.lastConversation = req
	for _, thought := range r.routeThoughts {
		if req.Notifier != nil {
			req.Notifier.OnThought(thought)
		}
	}
	if r.routeWaitCancel {
		if r.routeStarted != nil {
			r.routeStartOnce.Do(func() { close(r.routeStarted) })
		}
		<-ctx.Done()
		return "", ctx.Err()
	}
	if r.routeErr != nil {
		return "", r.routeErr
	}
	return "ok", nil
}

func (r *runTestRouter) HandleSessionAction(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

func (r *runTestRouter) HandleSessionActionTyped(ctx context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	r.sessionActions = append(r.sessionActions, req)
	switch req.Action {
	case "new":
		sessionID := firstNonEmpty(r.newSessionID, "logical-eval")
		return middleware.SessionActionResult{
			Action:          "new",
			ActiveSessionID: sessionID,
			Session: &middleware.SessionEntry{
				LogicalSessionID: sessionID,
				AgentID:          req.Target,
				WorkspacePath:    req.WorkspacePath,
				Ephemeral:        req.Ephemeral,
				CleanupPolicy:    req.CleanupPolicy,
				Active:           true,
			},
		}, nil
	case "status":
		if len(r.statusQueue) > 0 {
			entry := r.statusQueue[0]
			r.statusQueue = r.statusQueue[1:]
			return middleware.SessionActionResult{
				Action:          "status",
				ActiveSessionID: entry.LogicalSessionID,
				Session:         &entry,
			}, nil
		}
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
	case "list":
		if len(r.listQueue) > 0 {
			entries := r.listQueue[0]
			r.listQueue = r.listQueue[1:]
			return middleware.SessionActionResult{
				Action:   "list",
				Sessions: entries,
			}, nil
		}
		return middleware.SessionActionResult{Action: "list"}, nil
	case "cleanup":
		if r.cleanupCtxErr != nil {
			r.cleanupCtxErr <- ctx.Err()
		}
		if cleanup, ok := r.cleanupByTarget[req.Target]; ok {
			cleanup = cloneSessionCleanupResult(cleanup)
			if cleanup.LogicalSessionID == "" {
				cleanup.LogicalSessionID = req.Target
			}
			if cleanup.AgentID == "" {
				cleanup.AgentID = "opencode"
			}
			if cleanup.ProtocolKind == "" {
				cleanup.ProtocolKind = "acp"
			}
			if cleanup.CleanupPolicy == "" {
				cleanup.CleanupPolicy = req.CleanupPolicy
			}
			return middleware.SessionActionResult{Action: "cleanup", Cleanup: &cleanup}, nil
		}
		return middleware.SessionActionResult{
			Action: "cleanup",
			Cleanup: &middleware.SessionCleanupResult{
				LogicalSessionID:        req.Target,
				RemoteSessionID:         "remote-eval",
				AgentID:                 "opencode",
				ProtocolKind:            "acp",
				CleanupPolicy:           req.CleanupPolicy,
				Clean:                   true,
				StrongCleanup:           true,
				CleanupStrength:         sessioncleanup.StrengthStrong,
				RemoteDeleteAttempted:   true,
				RemoteDeleteUnsupported: true,
				RemoteCancelAttempted:   true,
				RemoteCanceled:          true,
				ProcessReapAttempted:    true,
				ProcessReaped:           true,
				LocalForgotten:          true,
			},
		}, nil
	case "reconcile":
		if r.reconcileErr != nil {
			return middleware.SessionActionResult{}, r.reconcileErr
		}
		return middleware.SessionActionResult{
			Action:    "reconcile",
			Reconcile: r.reconcile,
		}, nil
	default:
		return middleware.SessionActionResult{Action: req.Action}, nil
	}
}

func cloneSessionCleanupResult(in middleware.SessionCleanupResult) middleware.SessionCleanupResult {
	out := in
	if in.ForkChildren != nil {
		out.ForkChildren = make([]middleware.SessionCleanupResult, len(in.ForkChildren))
		for i, child := range in.ForkChildren {
			out.ForkChildren[i] = cloneSessionCleanupResult(child)
		}
	}
	if in.RelatedSessions != nil {
		out.RelatedSessions = append([]middleware.SessionCleanupRelatedSession(nil), in.RelatedSessions...)
	}
	if in.Warnings != nil {
		out.Warnings = append([]string(nil), in.Warnings...)
	}
	return out
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

func (r *runTestRouter) AttachRunContext(_ context.Context, req middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
	r.attachMu.Lock()
	defer r.attachMu.Unlock()
	r.attachRequests = append(r.attachRequests, req)
	if req.Notifier != nil {
		req.Notifier.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeThinking, Content: "live attach accepted by provider"})
	}
	return middleware.RunContextAttachmentResult{
		Action:     "attach_context",
		Status:     "delivered",
		DeliveryID: req.DeliveryID,
		Message:    "delivered",
	}, nil
}

func TestHandleRuns_NewEphemeralDeleteAfterRunCreatesCleansAndTraces(t *testing.T) {
	router := &runTestRouter{
		reconcile: &middleware.AgentClientReconcileResult{
			Reaped: []middleware.AgentClientRef{{AgentID: "opencode", WorkspacePath: "/tmp/eval-ws"}},
		},
	}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":             "noema-eval-channel-random",
		"agent_id":               "opencode",
		"input":                  "run eval",
		"workspace_path":         "/tmp/eval-ws",
		"additional_directories": []string{"/tmp/eval-lib", "/tmp/eval-lib"},
		"session_policy":         middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy":         middleware.SessionCleanupPolicyDeleteRemoteOrForgetLocal,
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
	if got := router.lastConversation.AdditionalDirectories; len(got) != 1 || got[0] != "/tmp/eval-lib" {
		t.Fatalf("expected normalized additional directories routed, got %#v", got)
	}
	if len(router.sessionActions) < 3 {
		t.Fatalf("expected new/list/cleanup actions, got %+v", router.sessionActions)
	}
	if router.sessionActions[0].Action != "list" || !router.sessionActions[0].LocalOnly {
		t.Fatalf("expected initial local-only list snapshot first, got %+v", router.sessionActions[0])
	}
	for _, action := range router.sessionActions {
		if action.Action == "list" && !action.LocalOnly {
			t.Fatalf("run cleanup snapshots must use local-only session lists, got %+v", action)
		}
	}
	newAction := firstSessionAction(router.sessionActions, "new")
	if newAction == nil || !newAction.Ephemeral {
		t.Fatalf("expected ephemeral new action, got %+v", router.sessionActions)
	}
	if got := newAction.AdditionalDirectories; len(got) != 1 || got[0] != "/tmp/eval-lib" {
		t.Fatalf("expected additional directories on prepared session action, got %#v", got)
	}
	cleanupAction := lastSessionAction(router.sessionActions, "cleanup")
	if cleanupAction == nil || !cleanupAction.ForceForgetLocal {
		t.Fatalf("expected force cleanup action, got %+v", router.sessionActions)
	}

	var resp runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if newAction.OwnerRunID != resp.RunID {
		t.Fatalf("ephemeral run session must carry owner run id, got %q want %q", newAction.OwnerRunID, resp.RunID)
	}
	if resp.Cleanup == nil || !resp.Cleanup.RemoteCanceled || !resp.Cleanup.LocalForgotten {
		t.Fatalf("expected cleanup proof in response, got %+v", resp.Cleanup)
	}
	if len(resp.Cleanup.RelatedSessions) != 1 || resp.Cleanup.RelatedSessions[0].Reason != sessioncleanup.ReasonRunUnreferencedAgentClientReaped {
		t.Fatalf("expected reconciled client reap proof in cleanup, got %+v", resp.Cleanup.RelatedSessions)
	}
	events, err := server.Store().LoadEvents(resp.RunID, 100)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if !hasEventKind(events, "session.policy.applied") || !hasEventKind(events, "session.cleanup") {
		t.Fatalf("expected policy and cleanup events, got %+v", events)
	}
}

func TestHandleRuns_EphemeralCleanupFailsRetainedReconciledClient(t *testing.T) {
	router := &runTestRouter{
		reconcile: &middleware.AgentClientReconcileResult{
			Retained: []middleware.AgentClientRef{{
				LogicalSessionID: "retained-logical",
				RemoteSessionID:  "retained-remote",
				AgentID:          "opencode",
				ProtocolKind:     "acp",
				WorkspacePath:    "/tmp/eval-ws",
			}},
		},
	}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "noema-eval-channel-retained-client",
		"agent_id":       "opencode",
		"input":          "run eval",
		"workspace_path": "/tmp/eval-ws",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected cleanup failure 500, got %d: %s", w.Code, w.Body.String())
	}
	var resp runresponse.Error
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Cleanup == nil || resp.Cleanup.Clean || resp.Cleanup.CleanupStrength != sessioncleanup.StrengthFailed {
		t.Fatalf("retained reconcile client must fail cleanup proof, got %+v", resp.Cleanup)
	}
	if resp.Cleanup.FailureCode != sessioncleanup.FailureRunRelatedSessionRetained {
		t.Fatalf("expected retained reconcile failure code, got %+v", resp.Cleanup)
	}
	if len(resp.Cleanup.RelatedSessions) != 1 {
		t.Fatalf("expected retained reconciled client in related sessions, got %+v", resp.Cleanup.RelatedSessions)
	}
	related := resp.Cleanup.RelatedSessions[0]
	if !related.Retained || related.AgentID != "opencode" || related.WorkspacePath != "/tmp/eval-ws" {
		t.Fatalf("expected retained opencode client proof, got %+v", related)
	}
	if related.LogicalSessionID != "retained-logical" || related.RemoteSessionID != "retained-remote" {
		t.Fatalf("expected retained client ownership details, got %+v", related)
	}
	cleanupEvent := waitForEvent(t, server.Store(), resp.RunID, "session.cleanup", runtrace.StatusFailed)
	if cleanupEvent.Metadata["failure_code"] != sessioncleanup.FailureRunRelatedSessionRetained {
		t.Fatalf("cleanup trace must expose retained reconcile failure, got %+v", cleanupEvent.Metadata)
	}
}

func TestHandleRuns_ProjectsStructuralToolEventsFromRouteNotifier(t *testing.T) {
	router := &runTestRouter{
		routeThoughts: []middleware.ThoughtUpdate{
			{
				Type:  middleware.ThoughtTypeToolCall,
				Title: "write_file",
				Metadata: map[string]interface{}{
					"source_update_type": "tool_call",
					"tool_call_id":       "native-tool-1",
					"tool_name":          "write_file",
					"tool_kind":          "edit",
					"status":             "pending",
					"raw_input": map[string]interface{}{
						"path": "/tmp/noema_matrix_contract.go",
					},
				},
			},
			{
				Type: middleware.ThoughtTypeToolResult,
				Metadata: map[string]interface{}{
					"source_update_type": "tool_call_update",
					"tool_call_id":       "native-tool-1",
					"tool_name":          "write_file",
					"tool_kind":          "edit",
					"status":             "completed",
					"raw_input": map[string]interface{}{
						"path": "/tmp/noema_matrix_contract.go",
					},
				},
			},
		},
	}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id": "noema-eval-channel-tools",
		"agent_id":   "opencode",
		"input":      "edit file",
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	trace, found, err := server.Store().Trace(resp.RunID)
	if err != nil || !found {
		t.Fatalf("trace found=%v err=%v", found, err)
	}
	var requested, completed *runtrace.Event
	for i := range trace.Events {
		switch trace.Events[i].Kind {
		case "tool.call.requested":
			requested = &trace.Events[i]
		case "tool.result.received":
			completed = &trace.Events[i]
		}
	}
	if requested == nil || completed == nil {
		t.Fatalf("expected tool events in public run trace: %#v", trace.Events)
	}
	if requested.ToolKind != "edit" || completed.Outputs["path"] != "/tmp/noema_matrix_contract.go" {
		t.Fatalf("unexpected structural projection: requested=%#v completed=%#v", requested, completed)
	}
}

func TestHandleRuns_EphemeralCleanupCleansRunCreatedActiveSessionWhenActiveChanges(t *testing.T) {
	router := &runTestRouter{
		newSessionID: "policy-session",
		listQueue: [][]middleware.SessionEntry{
			{{LogicalSessionID: "pre-existing", AgentID: "opencode", Active: true}},
			{{LogicalSessionID: "active-after-route", AgentID: "opencode", Active: true}},
			{{LogicalSessionID: "active-after-route", AgentID: "opencode", Active: true}},
		},
	}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "noema-eval-channel-active-switch",
		"agent_id":       "opencode",
		"input":          "run eval",
		"workspace_path": "/tmp/eval-ws",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	cleanupTargets := []string{}
	for _, action := range router.sessionActions {
		if action.Action == "cleanup" {
			cleanupTargets = append(cleanupTargets, action.Target)
		}
	}
	if strings.Join(cleanupTargets, ",") != "policy-session,active-after-route" {
		t.Fatalf("expected cleanup of policy and run-created active session, got %+v", cleanupTargets)
	}
	var resp runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Cleanup == nil || !resp.Cleanup.Clean || resp.Cleanup.CleanupStrength != sessioncleanup.StrengthStrong {
		t.Fatalf("run-created related session must be cleaned, got %+v", resp.Cleanup)
	}
	if len(resp.Cleanup.RelatedSessions) != 1 || resp.Cleanup.RelatedSessions[0].LogicalSessionID != "active-after-route" ||
		resp.Cleanup.RelatedSessions[0].Retained {
		t.Fatalf("expected non-retained related session cleanup proof, got %+v", resp.Cleanup.RelatedSessions)
	}
}

func TestHandleRuns_EphemeralCleanupFailsPreExistingActiveSessionWhenActiveChanges(t *testing.T) {
	router := &runTestRouter{
		newSessionID: "policy-session",
		listQueue: [][]middleware.SessionEntry{
			{{LogicalSessionID: "pre-existing", AgentID: "opencode", Active: true}},
			{{LogicalSessionID: "pre-existing", AgentID: "opencode", Active: true}},
			{{LogicalSessionID: "pre-existing", AgentID: "opencode", Active: true}},
		},
	}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "noema-eval-channel-active-switch-preexisting",
		"agent_id":       "opencode",
		"input":          "run eval",
		"workspace_path": "/tmp/eval-ws",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected cleanup failure 500, got %d: %s", w.Code, w.Body.String())
	}
	cleanup := lastSessionAction(router.sessionActions, "cleanup")
	if cleanup == nil || cleanup.Target != "policy-session" {
		t.Fatalf("cleanup must target prepared policy session, got %+v", cleanup)
	}
	var resp runresponse.Error
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Cleanup == nil || resp.Cleanup.Clean || resp.Cleanup.CleanupStrength != "failed" || !resp.Cleanup.ProcessRetained {
		t.Fatalf("retained pre-existing related session must fail cleanup proof, got %+v", resp.Cleanup)
	}
	if resp.Cleanup.FailureCode != sessioncleanup.FailureRunRelatedSessionRetained {
		t.Fatalf("expected retained-session failure code, got %+v", resp.Cleanup)
	}
	if len(resp.Cleanup.RelatedSessions) != 1 || resp.Cleanup.RelatedSessions[0].LogicalSessionID != "pre-existing" {
		t.Fatalf("expected retained pre-existing related session in cleanup proof, got %+v", resp.Cleanup.RelatedSessions)
	}
	cleanupEvent := waitForEvent(t, server.Store(), resp.RunID, "session.cleanup", runtrace.StatusFailed)
	if cleanupEvent.Metadata["process_retained"] != true {
		t.Fatalf("cleanup trace must account for retained related session, got %+v", cleanupEvent.Metadata)
	}
	if cleanupEvent.Metadata["clean"] != false || cleanupEvent.Metadata["failure_code"] != sessioncleanup.FailureRunRelatedSessionRetained {
		t.Fatalf("cleanup trace must fail retained related session, got %+v", cleanupEvent.Metadata)
	}
}

func TestHandleRuns_EphemeralCleanupIncludesNewOwnedRelatedSessions(t *testing.T) {
	router := &runTestRouter{
		newSessionID: "policy-session",
		statusQueue: []middleware.SessionEntry{
			{LogicalSessionID: "pre-existing", AgentID: "opencode", Active: true},
			{LogicalSessionID: "policy-session", AgentID: "opencode", Active: true},
		},
		listQueue: [][]middleware.SessionEntry{
			{{LogicalSessionID: "pre-existing", AgentID: "opencode", Active: true}},
			{{LogicalSessionID: "policy-session", AgentID: "opencode", Ephemeral: true, CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal, Active: true}},
			{
				{LogicalSessionID: "pre-existing", AgentID: "opencode"},
				{LogicalSessionID: "policy-session", AgentID: "opencode", Ephemeral: true, CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal, Active: true},
				{LogicalSessionID: "fork-child", AgentID: "opencode", Ephemeral: true, CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal, ParentSessionID: "pre-existing"},
			},
		},
	}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "noema-eval-channel-fork-child",
		"agent_id":       "opencode",
		"input":          "run eval",
		"workspace_path": "/tmp/eval-ws",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	cleanupTargets := []string{}
	for _, action := range router.sessionActions {
		if action.Action == "cleanup" {
			cleanupTargets = append(cleanupTargets, action.Target)
		}
	}
	if strings.Join(cleanupTargets, ",") != "policy-session,fork-child" {
		t.Fatalf("expected cleanup of policy and owned related session, got %+v", cleanupTargets)
	}
	var resp runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Cleanup == nil || len(resp.Cleanup.RelatedSessions) != 1 {
		t.Fatalf("expected related session cleanup proof, got %+v", resp.Cleanup)
	}
	related := resp.Cleanup.RelatedSessions[0]
	if related.LogicalSessionID != "fork-child" || related.Retained {
		t.Fatalf("expected cleaned related fork child, got %+v", related)
	}
}

func TestHandleRuns_EphemeralCleanupDoesNotRecleanForkChildrenCoveredByParentCleanup(t *testing.T) {
	router := &runTestRouter{
		newSessionID: "policy-session",
		statusQueue: []middleware.SessionEntry{
			{LogicalSessionID: "pre-existing", AgentID: "opencode", Active: true},
			{LogicalSessionID: "policy-session", AgentID: "opencode", Active: true},
		},
		listQueue: [][]middleware.SessionEntry{
			{{LogicalSessionID: "pre-existing", AgentID: "opencode", Active: true}},
			{{LogicalSessionID: "policy-session", AgentID: "opencode", Ephemeral: true, CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal, Active: true}},
			{
				{LogicalSessionID: "pre-existing", AgentID: "opencode"},
				{LogicalSessionID: "policy-session", AgentID: "opencode", Ephemeral: true, CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal, Active: true},
				{LogicalSessionID: "fork-child", RemoteSessionID: "fork-remote", AgentID: "opencode", Ephemeral: true, CleanupPolicy: middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal, ParentSessionID: "policy-session"},
			},
		},
		cleanupByTarget: map[string]middleware.SessionCleanupResult{
			"policy-session": {
				LogicalSessionID:     "policy-session",
				RemoteSessionID:      "policy-remote",
				AgentID:              "opencode",
				ProtocolKind:         "acp",
				Clean:                true,
				StrongCleanup:        true,
				CleanupStrength:      sessioncleanup.StrengthStrong,
				ProcessReapAttempted: true,
				ProcessReaped:        true,
				LocalForgotten:       true,
				ForkChildrenCleaned:  1,
				ForkChildren: []middleware.SessionCleanupResult{
					{
						LogicalSessionID: "fork-child",
						RemoteSessionID:  "fork-remote",
						AgentID:          "opencode",
						ProtocolKind:     "acp",
						Clean:            true,
						StrongCleanup:    true,
						CleanupStrength:  sessioncleanup.StrengthStrong,
						ProcessReaped:    true,
						LocalForgotten:   true,
					},
				},
			},
		},
	}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "noema-eval-channel-fork-child-covered",
		"agent_id":       "opencode",
		"input":          "run eval",
		"workspace_path": "/tmp/eval-ws",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	cleanupTargets := []string{}
	for _, action := range router.sessionActions {
		if action.Action == "cleanup" {
			cleanupTargets = append(cleanupTargets, action.Target)
		}
	}
	if strings.Join(cleanupTargets, ",") != "policy-session" {
		t.Fatalf("parent cleanup already covered fork child; got cleanup targets %+v", cleanupTargets)
	}
	var resp runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Cleanup == nil || !resp.Cleanup.Clean || !resp.Cleanup.StrongCleanup {
		t.Fatalf("expected strong cleanup response, got %+v", resp.Cleanup)
	}
	if len(resp.Cleanup.RelatedSessions) != 1 {
		t.Fatalf("expected covered fork child related proof, got %+v", resp.Cleanup.RelatedSessions)
	}
	related := resp.Cleanup.RelatedSessions[0]
	if related.LogicalSessionID != "fork-child" || related.Retained || related.Reason != "run_related_session_cleaned" {
		t.Fatalf("expected non-retained covered fork child, got %+v", related)
	}
}

func TestHandleRuns_EphemeralCleanupDoesNotRecleanRelatedParentCoveredByChildCleanup(t *testing.T) {
	router := &runTestRouter{
		newSessionID: "fork-child",
		statusQueue: []middleware.SessionEntry{
			{},
			{LogicalSessionID: "fork-child", AgentID: "opencode", Active: true},
		},
		listQueue: [][]middleware.SessionEntry{
			nil,
			{
				{
					LogicalSessionID: "fork-child",
					RemoteSessionID:  "fork-remote",
					AgentID:          "opencode",
					Ephemeral:        true,
					CleanupPolicy:    middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
					ParentSessionID:  "fork-parent",
					ParentRemoteID:   "parent-remote",
					Active:           true,
				},
			},
			{
				{
					LogicalSessionID: "fork-child",
					RemoteSessionID:  "fork-remote",
					AgentID:          "opencode",
					Ephemeral:        true,
					CleanupPolicy:    middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
					ParentSessionID:  "fork-parent",
					ParentRemoteID:   "parent-remote",
					Active:           true,
				},
				{
					LogicalSessionID: "fork-parent",
					RemoteSessionID:  "parent-remote",
					AgentID:          "opencode",
					Ephemeral:        true,
					CleanupPolicy:    middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
				},
			},
		},
		cleanupByTarget: map[string]middleware.SessionCleanupResult{
			"fork-child": {
				LogicalSessionID:     "fork-child",
				RemoteSessionID:      "fork-remote",
				AgentID:              "opencode",
				ProtocolKind:         "acp",
				Clean:                true,
				StrongCleanup:        true,
				CleanupStrength:      sessioncleanup.StrengthStrong,
				RemoteCanceled:       true,
				ProcessReapAttempted: true,
				ProcessReaped:        true,
				LocalForgotten:       true,
				RelatedSessions: []middleware.SessionCleanupRelatedSession{
					{
						LogicalSessionID: "fork-parent",
						RemoteSessionID:  "parent-remote",
						AgentID:          "opencode",
						Ephemeral:        true,
						CleanupPolicy:    middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
						Retained:         false,
						Reason:           sessioncleanup.ReasonForkParentAgentClientOwner,
					},
				},
			},
		},
	}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "noema-eval-channel-covered-parent",
		"agent_id":       "opencode",
		"input":          "run eval",
		"workspace_path": "/tmp/eval-ws",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	cleanupTargets := []string{}
	for _, action := range router.sessionActions {
		if action.Action == "cleanup" {
			cleanupTargets = append(cleanupTargets, action.Target)
		}
	}
	if strings.Join(cleanupTargets, ",") != "fork-child" {
		t.Fatalf("child cleanup already covered related parent; got cleanup targets %+v", cleanupTargets)
	}
	var resp runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Cleanup == nil || !resp.Cleanup.Clean || !resp.Cleanup.StrongCleanup {
		t.Fatalf("expected strong cleanup response, got %+v", resp.Cleanup)
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
	var resp runresponse.Error
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v; body=%s", err, w.Body.String())
	}
	if resp.Cleanup == nil || !resp.Cleanup.Clean || !resp.Cleanup.LocalForgotten {
		t.Fatalf("expected cleanup proof in error response, got %+v", resp.Cleanup)
	}
}

func TestHandleRuns_RouteFailureAcceptsUnmaterializedProcessAbsenceProof(t *testing.T) {
	router := &runTestRouter{
		routeErr: errors.New("strict guidance render failed"),
		cleanupByTarget: map[string]middleware.SessionCleanupResult{
			"logical-eval": {
				LogicalSessionID:     "logical-eval",
				AgentID:              "opencode",
				ProtocolKind:         "acp",
				Clean:                true,
				StrongCleanup:        true,
				CleanupStrength:      sessioncleanup.StrengthStrong,
				ProcessReapAttempted: true,
				ProcessAbsent:        true,
				ProcessAbsenceReason: sessioncleanup.NoMatchingCachedAgentClient,
				LocalForgotten:       true,
			},
		},
	}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "eval-channel-route-fail-unmaterialized",
		"agent_id":       "opencode",
		"input":          "run eval",
		"workspace_path": "/tmp/eval-ws",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected route failure 500, got %d: %s", w.Code, w.Body.String())
	}
	var resp runresponse.Error
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v; body=%s", err, w.Body.String())
	}
	if resp.Cleanup == nil || !resp.Cleanup.Clean || !resp.Cleanup.StrongCleanup || !resp.Cleanup.ProcessAbsent {
		t.Fatalf("expected strong process absence cleanup proof, got %+v", resp.Cleanup)
	}
	if resp.Cleanup.FailureCode != "" || resp.Cleanup.ProcessAbsenceReason != sessioncleanup.NoMatchingCachedAgentClient {
		t.Fatalf("unexpected cleanup failure/absence reason: %+v", resp.Cleanup)
	}
	event := waitForEvent(t, server.Store(), resp.RunID, "session.cleanup", runtrace.StatusCompleted)
	if event.Metadata["process_absent"] != true || event.Metadata["process_absence_reason"] != sessioncleanup.NoMatchingCachedAgentClient {
		t.Fatalf("cleanup trace must expose process absence proof, got %+v", event.Metadata)
	}
}

func TestHandleRuns_ProviderFailureIsTyped(t *testing.T) {
	router := &runTestRouter{routeErr: &providerfailure.Failure{
		Code:           providerfailure.ModelUnavailable,
		Message:        "configured provider model is unavailable through the selected adapter",
		AgentID:        "codex",
		Protocol:       "acp",
		Phase:          "session/prompt",
		RequestedModel: "gpt-5.5",
		Diagnostics: map[string]string{
			"command":   "/home/jose/.nvm/versions/node/v22.12.0/bin/codex-acp",
			"adapter":   "codex-acp",
			"transport": "stdio",
		},
		Err: errors.New("RPC error -32603: model access denied"),
	}}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id": "eval-channel-model-mismatch",
		"agent_id":   "codex",
		"input":      "Respond with exactly OK",
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFailedDependency {
		t.Fatalf("expected 424, got %d: %s", w.Code, w.Body.String())
	}
	var resp runresponse.Error
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v; body=%s", err, w.Body.String())
	}
	if resp.Code != providerfailure.ModelUnavailable {
		t.Fatalf("expected typed provider code, got %+v", resp)
	}
	if resp.Details["requested_model"] != "gpt-5.5" || resp.Details["adapter"] != "codex-acp" {
		t.Fatalf("expected provider diagnostics, got %+v", resp.Details)
	}
	events, err := server.Store().LoadEvents(resp.RunID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	if !hasEventKind(events, "provider.preflight.failed") {
		t.Fatalf("expected provider preflight event, got %+v", events)
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

func TestHandleRuns_ActivityTimeoutIsExplicit(t *testing.T) {
	router := &runTestRouter{routeWaitCancel: true}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":               "activity-timeout",
		"agent_id":                 "opencode",
		"input":                    "wait for activity timeout",
		"activity_timeout_seconds": 1,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	started := time.Now()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected activity timeout 504, got %d: %s", w.Code, w.Body.String())
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("activity timeout took too long: %s", time.Since(started))
	}
	var resp runresponse.Error
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != runtrace.StatusCancelled || !strings.Contains(resp.Error, "run activity timeout") {
		t.Fatalf("expected cancelled activity timeout response, got %+v", resp)
	}
	run, found, err := server.Store().LoadRun(resp.RunID)
	if err != nil || !found {
		t.Fatalf("LoadRun: found=%v err=%v", found, err)
	}
	if run.Status != runtrace.StatusCancelled || run.StopReason != "activity_timeout" {
		t.Fatalf("expected activity timeout cancellation, got %+v", run)
	}
}

func TestHandleRunActionsCancelCleansEphemeralRunWithDetachedContext(t *testing.T) {
	var logs safeLogBuffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	router := &runTestRouter{
		routeWaitCancel: true,
		routeStarted:    make(chan struct{}),
		cleanupCtxErr:   make(chan error, 1),
	}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "eval-cancel-cleanup",
		"agent_id":       "opencode",
		"execution_mode": "async",
		"input":          "run until cancelled",
		"workspace_path": "/tmp/eval-ws",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 async run, got %d: %s", w.Code, w.Body.String())
	}
	var runResp runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &runResp); err != nil {
		t.Fatalf("decode run response: %v", err)
	}
	select {
	case <-router.routeStarted:
	case <-time.After(time.Second):
		t.Fatalf("route did not start")
	}

	actionBody, _ := json.Marshal(map[string]interface{}{
		"action": "cancel",
		"reason": "test_interrupt_resume",
	})
	actionReq := httptest.NewRequest(http.MethodPost, RunResourcePrefixV1+runResp.RunID+"/actions", bytes.NewReader(actionBody))
	actionW := httptest.NewRecorder()
	mux.ServeHTTP(actionW, actionReq)
	if actionW.Code != http.StatusAccepted {
		t.Fatalf("expected 202 cancel, got %d: %s", actionW.Code, actionW.Body.String())
	}

	cleanupEvent := waitForEvent(t, server.Store(), runResp.RunID, "session.cleanup", runtrace.StatusCompleted)
	if cleanupEvent.Metadata["clean"] != true || cleanupEvent.Metadata["remote_canceled"] != true || cleanupEvent.Metadata["process_reaped"] != true {
		t.Fatalf("expected clean cleanup proof, got %+v", cleanupEvent.Metadata)
	}
	select {
	case err := <-router.cleanupCtxErr:
		if err != nil {
			t.Fatalf("cleanup context must survive run cancel, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("cleanup context was not observed")
	}
	run, found, err := server.Store().LoadRun(runResp.RunID)
	if err != nil || !found {
		t.Fatalf("LoadRun: found=%v err=%v", found, err)
	}
	if run.Status != runtrace.StatusCancelled {
		t.Fatalf("expected cancelled run, got %+v", run)
	}
	waitForLog(t, &logs, "matrix async run cancelled")
	if strings.Contains(logs.String(), "matrix async run bridge failed") {
		t.Fatalf("expected async cancel to avoid generic bridge failure log, got %s", logs.String())
	}
}

func TestHandleRunActionsCancelRaceCleanupTracksLateSelectedRemoteSession(t *testing.T) {
	router := &runTestRouter{
		newSessionID:    "requested-session",
		routeWaitCancel: true,
		routeStarted:    make(chan struct{}),
		listQueue: [][]middleware.SessionEntry{
			{},
			{{
				LogicalSessionID: "selected-session",
				RemoteSessionID:  "remote-session",
				AgentID:          "opencode",
				ProtocolKind:     "acp",
				WorkspacePath:    "/tmp/eval-ws",
				Status:           "active",
				Active:           true,
			}},
			{{
				LogicalSessionID: "selected-session",
				RemoteSessionID:  "remote-session",
				AgentID:          "opencode",
				ProtocolKind:     "acp",
				WorkspacePath:    "/tmp/eval-ws",
				Status:           "active",
				Active:           true,
			}},
		},
		cleanupByTarget: map[string]middleware.SessionCleanupResult{
			"requested-session": {
				LogicalSessionID:        "requested-session",
				AgentID:                 "opencode",
				ProtocolKind:            "acp",
				Clean:                   true,
				CleanupStrength:         sessioncleanup.StrengthRetained,
				ProcessRetained:         true,
				ProcessRetentionAllowed: true,
				ProcessRetentionReason:  sessioncleanup.OtherLocalSessionsStillReferenceAgentClient,
				LocalForgotten:          true,
			},
			"selected-session": {
				LogicalSessionID:      "selected-session",
				RemoteSessionID:       "remote-session",
				AgentID:               "opencode",
				ProtocolKind:          "acp",
				Clean:                 true,
				StrongCleanup:         true,
				CleanupStrength:       sessioncleanup.StrengthStrong,
				RemoteCancelAttempted: true,
				RemoteCanceled:        true,
				ProcessReapAttempted:  true,
				ProcessReaped:         true,
				LocalForgotten:        true,
			},
		},
	}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":     "eval-cancel-create-race",
		"agent_id":       "opencode",
		"execution_mode": "async",
		"input":          "run until cancelled",
		"workspace_path": "/tmp/eval-ws",
		"session_policy": middleware.SessionPolicyNewEphemeralDeleteAfterRun,
		"cleanup_policy": middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 async run, got %d: %s", w.Code, w.Body.String())
	}
	var runResp runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &runResp); err != nil {
		t.Fatalf("decode run response: %v", err)
	}
	select {
	case <-router.routeStarted:
	case <-time.After(time.Second):
		t.Fatalf("route did not start")
	}
	if router.lastConversation.LogicalSessionID != "requested-session" {
		t.Fatalf("run route must bind the prepared ephemeral session, got %+v", router.lastConversation)
	}

	actionBody, _ := json.Marshal(map[string]interface{}{"action": "cancel", "reason": "test_create_race"})
	actionReq := httptest.NewRequest(http.MethodPost, RunResourcePrefixV1+runResp.RunID+"/actions", bytes.NewReader(actionBody))
	actionW := httptest.NewRecorder()
	mux.ServeHTTP(actionW, actionReq)
	if actionW.Code != http.StatusAccepted {
		t.Fatalf("expected 202 cancel, got %d: %s", actionW.Code, actionW.Body.String())
	}

	cleanupEvent := waitForEvent(t, server.Store(), runResp.RunID, "session.cleanup", runtrace.StatusCompleted)
	if cleanupEvent.Metadata["logical_session_id"] != "selected-session" ||
		cleanupEvent.Metadata["remote_session_id"] != "remote-session" ||
		cleanupEvent.Metadata["strong_cleanup"] != true ||
		cleanupEvent.Metadata["cleanup_strength"] != sessioncleanup.StrengthStrong ||
		cleanupEvent.Metadata["process_retained"] == true {
		t.Fatalf("cleanup must target late selected remote session strongly, got %+v", cleanupEvent.Metadata)
	}
	for _, action := range router.sessionActions {
		if action.Action == "cleanup" && action.Target == "requested-session" {
			t.Fatalf("cancel race cleanup must not target stale requested session: %+v", router.sessionActions)
		}
	}
}

func TestHandleRuns_SidecarCapsulesAreNeutralAndTraced(t *testing.T) {
	router := &runTestRouter{}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id": "noema.http",
		"agent_id":   "opencode",
		"input": map[string]interface{}{
			"text": "update parser",
		},
		"sidecar_capsules": []map[string]interface{}{
			{
				"provider":   "noema",
				"id":         "caps_7f31",
				"schema":     "sidecar.intent.v0",
				"version":    "0.1",
				"visibility": "llm_visible",
				"format":     "noema_xml",
				"content":    "<noema id=\"caps_7f31\">intent: evolve_config_parser</noema>",
				"metadata": map[string]interface{}{
					"intent": "evolve_config_parser",
				},
			},
		},
		"trace_policy": map[string]interface{}{
			"content_mode":          runtrace.ContentModeRefs,
			"include_protocol_meta": false,
		},
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastConversation.Input != "update parser" {
		t.Fatalf("expected clean task body routed separately, got %q", router.lastConversation.Input)
	}
	if len(router.lastConversation.SidecarCapsules) != 1 || router.lastConversation.SidecarCapsules[0].ID != "caps_7f31" {
		t.Fatalf("expected sidecar capsule on neutral conversation request, got %+v", router.lastConversation.SidecarCapsules)
	}

	var resp runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	trace, found, err := server.Store().Trace(resp.RunID)
	if err != nil || !found {
		t.Fatalf("Trace found=%v err=%v", found, err)
	}
	var sidecar *runtrace.Event
	for i := range trace.Events {
		if trace.Events[i].Kind == "sidecar.capsule.delivered" {
			sidecar = &trace.Events[i]
			break
		}
	}
	if sidecar == nil {
		t.Fatalf("expected sidecar delivery event, got %+v", trace.Events)
	}
	if sidecar.SidecarProvider != "noema" || sidecar.SidecarID != "caps_7f31" || sidecar.SidecarVisibility != "llm_visible" {
		t.Fatalf("unexpected sidecar identity: %+v", sidecar)
	}
	if sidecar.Message != "" {
		t.Fatalf("sidecar content must not be inline in refs mode, got %q", sidecar.Message)
	}
	if sidecar.ProtocolMeta != nil {
		t.Fatalf("protocol meta should be hidden by trace policy, got %+v", sidecar.ProtocolMeta)
	}
	if sidecar.Metadata["frontend_visible"] != false || sidecar.Metadata["audit_visible"] != true {
		t.Fatalf("expected frontend-hidden audit-visible event metadata, got %+v", sidecar.Metadata)
	}
}

func TestHandleRuns_SidecarValidationRejectsInvisibleContentWithoutID(t *testing.T) {
	router := &runTestRouter{}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id": "bad-sidecar",
		"input":      "task",
		"sidecar_capsules": []map[string]interface{}{
			{
				"provider":   "noema",
				"visibility": "llm_visible",
				"content":    "<noema/>",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunsRejectsRelativeAdditionalDirectories(t *testing.T) {
	router := &runTestRouter{}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"channel_id":             "bad-additional-dirs",
		"input":                  "task",
		"additional_directories": []string{"relative"},
	})
	req := httptest.NewRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "absolute") {
		t.Fatalf("expected absolute path validation message, got %q", w.Body.String())
	}
}

func TestHandleRunActions_AttachContextDeliversToActiveRunSession(t *testing.T) {
	router := &runTestRouter{}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	run, _, err := server.Store().Start(runtrace.Run{
		AgentID:          "opencode",
		Protocol:         "acp",
		ChannelID:        "noema.http",
		ExecutionMode:    runtrace.ExecutionModeAsync,
		LogicalSessionID: "logical-live",
		RemoteSessionID:  "remote-live",
		TracePolicy:      runtrace.TracePolicy{ContentMode: runtrace.ContentModeInline},
	})
	if err != nil {
		t.Fatalf("Start run: %v", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"action":          "attach_context",
		"reason":          "supervisor_suggestion",
		"source_event_id": "evt-source",
		"sidecar_capsules": []map[string]interface{}{
			{
				"provider":   "noema",
				"id":         "sug_01",
				"schema":     "noema.sidecar.suggestion.v0",
				"version":    "0.1",
				"visibility": "llm_visible",
				"format":     "noema_xml",
				"content":    "<noema-suggestion>avoid loop</noema-suggestion>",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, RunResourcePrefixV1+run.ID+"/actions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	deliveryID, _ := resp["delivery_id"].(string)
	if deliveryID == "" || resp["accepted"] != true {
		t.Fatalf("expected accepted delivery response, got %+v", resp)
	}
	attached := waitForEvent(t, server.Store(), run.ID, "run.context.attached", "delivered")
	if attached.Metadata["delivery_id"] != deliveryID || attached.Metadata["source_event_id"] != "evt-source" {
		t.Fatalf("unexpected attach metadata: %+v", attached.Metadata)
	}
	if attached.Message != "delivered" || attached.Summary != "Live context delivered to active session." {
		t.Fatalf("expected inline delivery proof without summary leak, got message=%q summary=%q", attached.Message, attached.Summary)
	}
	delivered := waitForEvent(t, server.Store(), run.ID, "sidecar.capsule.delivered", runtrace.StatusCompleted)
	if delivered.SidecarID != "sug_01" || delivered.Metadata["delivery_id"] != deliveryID {
		t.Fatalf("unexpected sidecar delivery event: %+v", delivered)
	}
	router.attachMu.Lock()
	defer router.attachMu.Unlock()
	if len(router.attachRequests) != 1 || router.attachRequests[0].LogicalSessionID != "logical-live" || router.attachRequests[0].RemoteSessionID != "remote-live" {
		t.Fatalf("expected exact active session attach, got %+v", router.attachRequests)
	}
	if router.attachRequests[0].Notifier == nil {
		t.Fatalf("expected attach notifier for live delivery trace")
	}
}

func TestHandleRunActions_AttachContextUnsupportedWhenSessionNotReady(t *testing.T) {
	router := &runTestRouter{}
	server := NewServer(router).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	run, _, err := server.Store().Start(runtrace.Run{AgentID: "opencode", Protocol: "acp", ChannelID: "noema.http", ExecutionMode: runtrace.ExecutionModeAsync})
	if err != nil {
		t.Fatalf("Start run: %v", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"action": "attach_context",
		"sidecar_capsules": []map[string]interface{}{
			{"provider": "noema", "id": "sug_unsupported", "visibility": "trace_only"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, RunResourcePrefixV1+run.ID+"/actions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 unsupported, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "unsupported" || resp["accepted"] != false {
		t.Fatalf("expected typed unsupported response, got %+v", resp)
	}
	event := waitForEvent(t, server.Store(), run.ID, "run.context.attached", "unsupported")
	if event.Metadata["delivery_status"] != "unsupported" {
		t.Fatalf("expected unsupported evidence, got %+v", event.Metadata)
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

func waitForEvent(t *testing.T, store *runtrace.Store, runID, kind, status string) runtrace.Event {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		events, err := store.LoadEvents(runID, 100)
		if err != nil {
			t.Fatalf("LoadEvents: %v", err)
		}
		for _, event := range events {
			if event.Kind == kind && event.Status == status {
				return event
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("event %s/%s not found", kind, status)
	return runtrace.Event{}
}

func waitForLog(t *testing.T, logs interface{ String() string }, fragment string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logs.String(), fragment) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("log fragment %q not found in %s", fragment, logs.String())
}

func hasSessionAction(actions []middleware.SessionActionRequest, action string) bool {
	return firstSessionAction(actions, action) != nil
}

func firstSessionAction(actions []middleware.SessionActionRequest, action string) *middleware.SessionActionRequest {
	for _, req := range actions {
		if req.Action == action {
			return &req
		}
	}
	return nil
}

func lastSessionAction(actions []middleware.SessionActionRequest, action string) *middleware.SessionActionRequest {
	for i := len(actions) - 1; i >= 0; i-- {
		if actions[i].Action == action {
			return &actions[i]
		}
	}
	return nil
}
