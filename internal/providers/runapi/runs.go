package runapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/middleware"
)

type runExecution struct {
	runID            string
	req              runRequest
	agentID          string
	emergencyTimeout time.Duration
}

func (s *Server) HandleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	req, ok := decodeRunRequest(w, r)
	if !ok {
		return
	}
	agentID := firstNonEmpty(req.AgentID, s.defaultAgent)
	run, err := s.startRun(req, agentID)
	if err != nil {
		slog.Error("matrix run trace start failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	s.dispatchByExecutionMode(w, r, runExecution{
		runID:            run.ID,
		req:              req,
		agentID:          agentID,
		emergencyTimeout: emergencyTimeout(req),
	})
}

func decodeRunRequest(w http.ResponseWriter, r *http.Request) (runRequest, bool) {
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return runRequest{}, false
	}
	if req.ChannelID == "" || req.Input == "" {
		http.Error(w, "Bad Request: channel_id and input are required", http.StatusBadRequest)
		return runRequest{}, false
	}
	return req, true
}

func (s *Server) startRun(req runRequest, agentID string) (runtrace.Run, error) {
	run, _, err := s.runStore.Start(runtrace.Run{
		AgentID:       agentID,
		Protocol:      s.resolveProtocol(agentID),
		WorkspaceID:   req.WorkspaceID,
		WorkspacePath: req.WorkspacePath,
		ChannelID:     req.ChannelID,
		ExecutionMode: normalizeRunExecutionMode(req.ExecutionMode),
		InputRef:      "matrix://pending/input",
		InputDigest:   runtrace.DigestString(req.Input),
		Context:       req.Context,
		ClientMeta:    req.ClientMeta,
		TracePolicy:   req.TracePolicy,
	})
	if err != nil {
		return runtrace.Run{}, err
	}
	run.InputRef = "matrix://runs/" + run.ID + "/input"
	if err := s.runStore.SaveRun(run); err != nil {
		return runtrace.Run{}, err
	}
	s.appendRouteEvents(run, req.AgentID, agentID)
	return run, nil
}

func (s *Server) appendRouteEvents(run runtrace.Run, requestedAgentID, selectedAgentID string) {
	_, _ = s.runStore.AppendEvent(runtrace.Event{
		RunID:        run.ID,
		Kind:         "routing.decision",
		Actor:        "matrix",
		Status:       runtrace.StatusCompleted,
		Protocol:     run.Protocol,
		DecisionID:   "decision-" + run.ID,
		ProtocolMeta: map[string]interface{}{"requested_agent_id": requestedAgentID, "selected_agent_id": selectedAgentID},
	})
	_, _ = s.runStore.AppendEvent(runtrace.Event{
		RunID:          run.ID,
		Kind:           "agent.prompt.sent",
		Actor:          "matrix",
		Status:         runtrace.StatusCompleted,
		Protocol:       run.Protocol,
		ProtocolMethod: "matrix.route",
		ContentRef:     run.InputRef,
		ContentDigest:  run.InputDigest,
	})
}

func (s *Server) dispatchByExecutionMode(w http.ResponseWriter, r *http.Request, exec runExecution) {
	switch normalizeRunExecutionMode(exec.req.ExecutionMode) {
	case runtrace.ExecutionModeAsync:
		ctx, cancel := executionContext(context.Background(), exec.emergencyTimeout)
		s.trackRunCancel(exec.runID, cancel)
		go s.executeRunAsync(ctx, exec)
		writeJSON(w, http.StatusAccepted, newRunResponse(exec.runID, runtrace.StatusRunning, ""))
	case runtrace.ExecutionModeStream:
		s.handleRunStream(w, r, exec)
	default:
		ctx, cancel := executionContext(r.Context(), exec.emergencyTimeout)
		defer cancel()
		res, err := s.executeRun(ctx, exec)
		if err != nil {
			slog.Error("matrix run bridge failed", "error", err)
			if isSetupRequired(err) {
				writeJSON(w, http.StatusConflict, map[string]string{
					"code":    "SETUP_REQUIRED",
					"message": "Matrix setup is required before non-interactive /v1/runs routing.",
					"hint":    "Run `matrix bootstrap doctor` and complete setup, or set system.configured only after provisioning the required agents.",
					"run_id":  exec.runID,
				})
				return
			}
			if isEmergencyTimeout(ctx, err, exec) {
				http.Error(w, "Gateway Timeout: emergency kill deadline reached", http.StatusGatewayTimeout)
				return
			}
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, newRunResponse(exec.runID, runtrace.StatusCompleted, res))
	}
}

func (s *Server) executeRun(ctx context.Context, exec runExecution) (string, error) {
	before := s.sessionSnapshot(ctx, exec.req.ChannelID, exec.req.WorkspaceID)
	notifier := &runTraceNotifier{store: s.runStore, runID: exec.runID, agentID: exec.agentID, protocol: s.resolveProtocol(exec.agentID)}
	res, err := s.route(ctx, exec.req, exec.agentID, notifier)
	if err != nil {
		if isEmergencyTimeout(ctx, err, exec) {
			_, _ = s.runStore.Cancel(exec.runID, "emergency_kill_timeout")
		} else {
			_, _ = s.runStore.Fail(exec.runID, err)
		}
		s.untrackRunCancel(exec.runID)
		return "", err
	}
	s.enrichRunFromSession(ctx, sessionEnrichmentRequest{
		runID:       exec.runID,
		channelID:   exec.req.ChannelID,
		workspaceID: exec.req.WorkspaceID,
		before:      before,
	})
	_, err = s.runStore.Complete(exec.runID, res, "end_turn")
	s.untrackRunCancel(exec.runID)
	return res, err
}

func (s *Server) route(ctx context.Context, req runRequest, agentID string, notifier middleware.ThoughtNotifier) (string, error) {
	if richer, ok := s.router.(middleware.ConversationRequestRouter); ok {
		return richer.RouteConversation(ctx, middleware.ConversationRequest{
			ChannelID:      req.ChannelID,
			AgentID:        agentID,
			WorkspaceID:    req.WorkspaceID,
			WorkspacePath:  req.WorkspacePath,
			Input:          req.Input,
			Notifier:       notifier,
			NonInteractive: true,
		})
	}
	return s.router.Route(ctx, req.ChannelID, agentID, req.Input, notifier)
}

func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request, exec runExecution) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(newRunResponse(exec.runID, runtrace.StatusRunning, ""))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	ctx, cancel := executionContext(r.Context(), exec.emergencyTimeout)
	defer cancel()
	res, err := s.executeRun(ctx, exec)
	if err != nil {
		status := runtrace.StatusFailed
		if isEmergencyTimeout(ctx, err, exec) {
			status = runtrace.StatusCancelled
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"run_id": exec.runID, "status": status, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(newRunResponse(exec.runID, runtrace.StatusCompleted, res))
}

func (s *Server) executeRunAsync(ctx context.Context, exec runExecution) {
	if _, err := s.executeRun(ctx, exec); err != nil {
		slog.Error("matrix async run bridge failed", "error", err, "run_id", exec.runID)
	}
}

func executionContext(parent context.Context, emergencyTimeout time.Duration) (context.Context, context.CancelFunc) {
	if emergencyTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, emergencyTimeout)
}

func emergencyTimeout(req runRequest) time.Duration {
	if req.EmergencyKillSeconds <= 0 {
		return 0
	}
	return time.Duration(req.EmergencyKillSeconds) * time.Second
}

func isEmergencyTimeout(ctx context.Context, err error, exec runExecution) bool {
	return exec.emergencyTimeout > 0 && (errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded)
}

func isSetupRequired(err error) bool {
	return errors.Is(err, middleware.ErrSetupRequired)
}
