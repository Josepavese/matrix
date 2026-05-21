package runapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/logic/runactivity"
	"github.com/Josepavese/matrix/internal/logic/runnotifier"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/sidecar"
	"github.com/Josepavese/matrix/internal/middleware"
	runresponse "github.com/Josepavese/matrix/internal/providers/runapi/response"
)

var runResponseBuilder = runresponse.Builder{Prefix: RunResourcePrefixV1}

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
		emergencyTimeout: runactivity.DurationSeconds(req.EmergencyKillSeconds),
		activityTimeout:  runactivity.DurationSeconds(req.ActivityTimeoutSeconds),
	})
}

func decodeRunRequest(w http.ResponseWriter, r *http.Request) (runRequest, bool) {
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return runRequest{}, false
	}
	if strings.TrimSpace(req.ChannelID) == "" || strings.TrimSpace(req.Input.String()) == "" {
		http.Error(w, "Bad Request: channel_id and input are required", http.StatusBadRequest)
		return runRequest{}, false
	}
	req.SidecarCapsules = sidecar.NormalizeCapsules(req.SidecarCapsules)
	if err := sidecar.ValidateCapsules(req.SidecarCapsules); err != nil {
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
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
		InputDigest:   runtrace.DigestString(req.Input.String()),
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
	s.appendSidecarEvents(run, req.SidecarCapsules)
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
		ctx, cancel := runactivity.Context(context.Background(), exec.emergencyTimeout)
		s.trackRunCancel(exec.runID, cancel)
		go s.executeRunAsync(ctx, exec)
		writeJSON(w, http.StatusAccepted, runResponseBuilder.NewSuccess(exec.runID, runtrace.StatusRunning, ""))
	case runtrace.ExecutionModeStream:
		s.handleRunStream(w, r, exec)
	default:
		ctx, cancel := runactivity.Context(r.Context(), exec.emergencyTimeout)
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
			if runactivity.IsDeadline(ctx, err, exec.emergencyTimeout) {
				writeJSON(w, http.StatusGatewayTimeout, runResponseBuilder.NewError(exec.runID, runtrace.StatusCancelled, "emergency kill deadline reached", res.cleanup))
				return
			}
			if runactivity.IsTimeoutError(err) {
				writeJSON(w, http.StatusGatewayTimeout, runResponseBuilder.NewErrorForError(exec.runID, runtrace.StatusCancelled, err, res.cleanup))
				return
			}
			if status, ok := providerfailure.HTTPStatus(err); ok {
				writeJSON(w, status, runResponseBuilder.NewErrorForError(exec.runID, runtrace.StatusFailed, err, res.cleanup))
				return
			}
			writeJSON(w, http.StatusInternalServerError, runResponseBuilder.NewErrorForError(exec.runID, runtrace.StatusFailed, err, res.cleanup))
			return
		}
		writeJSON(w, http.StatusCreated, runResponseBuilder.NewSuccess(exec.runID, runtrace.StatusCompleted, res.output, res.cleanup))
	}
}

func (s *Server) executeRun(ctx context.Context, exec runExecution) (runExecutionResult, error) {
	sessionCtx, err := s.prepareRunSessionContext(ctx, exec)
	if err != nil {
		_, _ = s.runStore.Fail(exec.runID, err)
		providerfailure.AppendRunEvent(s.runStore, exec.runID, err)
		s.untrackRunCancel(exec.runID)
		return runExecutionResult{}, err
	}
	notifier := runnotifier.New(s.runStore, exec.runID, exec.agentID, s.resolveProtocol(exec.agentID))
	routeCtx, routeNotifier, activityState, stopActivityWatch := runactivity.WithTimeout(ctx, exec.activityTimeout, notifier)
	defer stopActivityWatch()
	res, err := s.route(routeCtx, exec, sessionCtx.prepared, routeNotifier)
	if err != nil {
		postCtx, postCancel := postRunContext(ctx)
		defer postCancel()
		after := s.enrichRunFromSession(postCtx, sessionEnrichmentRequest{
			runID:       exec.runID,
			channelID:   exec.req.ChannelID,
			workspaceID: exec.req.WorkspaceID,
			before:      sessionCtx.before,
		})
		cleanup, _ := s.cleanupRunSessionContext(postCtx, exec, sessionCtx, after)
		switch {
		case runactivity.IsTimeout(activityState, err):
			err = runactivity.Error(activityState)
			_, _ = s.runStore.Cancel(exec.runID, "activity_timeout")
		case runactivity.IsDeadline(ctx, err, exec.emergencyTimeout):
			_, _ = s.runStore.Cancel(exec.runID, "emergency_kill_timeout")
		case runactivity.IsContextCancelled(ctx, err):
			_, _ = s.runStore.Cancel(exec.runID, "cancelled")
		default:
			_, _ = s.runStore.Fail(exec.runID, err)
			providerfailure.AppendRunEvent(s.runStore, exec.runID, err)
		}
		s.untrackRunCancel(exec.runID)
		return runExecutionResult{cleanup: cleanup}, err
	}
	after := s.enrichRunFromSession(ctx, sessionEnrichmentRequest{
		runID:       exec.runID,
		channelID:   exec.req.ChannelID,
		workspaceID: exec.req.WorkspaceID,
		before:      sessionCtx.before,
	})
	cleanup, err := s.cleanupRunSessionContext(ctx, exec, sessionCtx, after)
	if err != nil {
		_, _ = s.runStore.Fail(exec.runID, err)
		s.untrackRunCancel(exec.runID)
		return runExecutionResult{output: res, cleanup: cleanup}, err
	}
	_, err = s.runStore.Complete(exec.runID, res, "end_turn")
	s.untrackRunCancel(exec.runID)
	return runExecutionResult{output: res, cleanup: cleanup}, err
}

func (s *Server) route(ctx context.Context, exec runExecution, prepared sessionSnapshot, notifier middleware.ThoughtNotifier) (string, error) {
	req := exec.req
	if richer, ok := s.router.(middleware.ConversationRequestRouter); ok {
		return richer.RouteConversation(ctx, middleware.ConversationRequest{
			ChannelID:        req.ChannelID,
			AgentID:          exec.agentID,
			LogicalSessionID: strings.TrimSpace(prepared.LogicalSessionID),
			WorkspaceID:      req.WorkspaceID,
			WorkspacePath:    req.WorkspacePath,
			Input:            req.Input.String(),
			SidecarCapsules:  req.SidecarCapsules,
			Notifier:         notifier,
			NonInteractive:   true,
		})
	}
	return s.router.Route(ctx, req.ChannelID, exec.agentID, sidecar.ProjectPrompt(req.Input.String(), req.SidecarCapsules), notifier)
}

func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request, exec runExecution) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(runResponseBuilder.NewSuccess(exec.runID, runtrace.StatusRunning, ""))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	ctx, cancel := runactivity.Context(r.Context(), exec.emergencyTimeout)
	defer cancel()
	res, err := s.executeRun(ctx, exec)
	if err != nil {
		status := runtrace.StatusFailed
		if runactivity.IsDeadline(ctx, err, exec.emergencyTimeout) || runactivity.IsTimeoutError(err) {
			status = runtrace.StatusCancelled
		}
		_ = json.NewEncoder(w).Encode(runResponseBuilder.NewErrorForError(exec.runID, status, err, res.cleanup))
		return
	}
	_ = json.NewEncoder(w).Encode(runResponseBuilder.NewSuccess(exec.runID, runtrace.StatusCompleted, res.output, res.cleanup))
}

func (s *Server) executeRunAsync(ctx context.Context, exec runExecution) {
	res, err := s.executeRun(ctx, exec)
	if err == nil {
		return
	}
	switch {
	case runactivity.IsDeadline(ctx, err, exec.emergencyTimeout):
		slog.Warn("matrix async run emergency timeout", append([]any{"event", "run_emergency_timeout", "error", err, "run_id", exec.runID}, cleanupLogArgs(res.cleanup)...)...)
	case runactivity.IsTimeoutError(err):
		slog.Warn("matrix async run activity timeout", append([]any{"event", "run_activity_timeout", "error", err, "run_id", exec.runID}, cleanupLogArgs(res.cleanup)...)...)
	case runactivity.IsContextCancelled(ctx, err):
		slog.Info("matrix async run cancelled", append([]any{"event", "run_cancelled", "error", err, "run_id", exec.runID}, cleanupLogArgs(res.cleanup)...)...)
	default:
		slog.Error("matrix async run bridge failed", append([]any{"error", err, "run_id", exec.runID}, cleanupLogArgs(res.cleanup)...)...)
	}
}

func postRunContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), runCleanupTimeout)
}

func isSetupRequired(err error) bool {
	return errors.Is(err, middleware.ErrSetupRequired)
}
