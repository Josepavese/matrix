package runapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jose/matrix-v2/internal/logic/memstore"
	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/middleware"
)

type runTestRouter struct {
	lastConversation middleware.ConversationRequest
	sessionActions   []middleware.SessionActionRequest
	attachMu         sync.Mutex
	attachRequests   []middleware.RunContextAttachmentRequest
	routeErr         error
	routeWaitCancel  bool
	routeStarted     chan struct{}
	routeStartOnce   sync.Once
	cleanupCtxErr    chan error
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
		if r.cleanupCtxErr != nil {
			r.cleanupCtxErr <- ctx.Err()
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

func (r *runTestRouter) AttachRunContext(_ context.Context, req middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
	r.attachMu.Lock()
	defer r.attachMu.Unlock()
	r.attachRequests = append(r.attachRequests, req)
	return middleware.RunContextAttachmentResult{
		Action:     "attach_context",
		Status:     "delivered",
		DeliveryID: req.DeliveryID,
		Message:    "delivered",
	}, nil
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

func TestHandleRunActionsCancelCleansEphemeralRunWithDetachedContext(t *testing.T) {
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
	var runResp runResponse
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

	var resp runResponse
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

func hasSessionAction(actions []middleware.SessionActionRequest, action string) bool {
	for _, req := range actions {
		if req.Action == action {
			return true
		}
	}
	return false
}
