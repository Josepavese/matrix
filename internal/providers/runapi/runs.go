package runapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/logic/runactivity"
	"github.com/Josepavese/matrix/internal/logic/runnotifier"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/sidecar"
	"github.com/Josepavese/matrix/internal/middleware"
	runresponse "github.com/Josepavese/matrix/internal/providers/runapi/response"
)

var runResponseBuilder = runresponse.Builder{Prefix: RunResourcePrefixV1}

func (s *Server) startRun(req runRequest, agentID, runID string) (runtrace.Run, error) {
	run, _, err := s.runStore.Start(runtrace.Run{
		ID:                runID,
		AgentID:           agentID,
		RequestedModel:    req.ModelID,
		ModelVerification: modelVerification(req.ModelID),
		Protocol:          s.resolveProtocol(agentID),
		WorkspaceID:       req.WorkspaceID,
		WorkspacePath:     req.WorkspacePath,
		ChannelID:         req.ChannelID,
		ExecutionMode:     normalizeRunExecutionMode(req.ExecutionMode),
		InputRef:          "matrix://pending/input",
		InputDigest:       runtrace.DigestString(req.Input.String()),
		Context:           req.Context,
		ClientMeta:        req.ClientMeta,
		TracePolicy:       req.TracePolicy,
	})
	if err != nil {
		return runtrace.Run{}, err
	}
	run.InputRef = "matrix://runs/" + run.ID + "/input"
	if err := s.runStore.SaveRun(run); err != nil {
		return runtrace.Run{}, err
	}
	s.appendRouteEvents(run, req.AgentID, agentID, req.agentLaunchArgs)
	s.appendSidecarEvents(run, req.SidecarCapsules)
	return run, nil
}

func modelVerification(modelID string) string {
	if modelID == "" {
		return ""
	}
	return "unverified"
}

func (s *Server) appendRouteEvents(run runtrace.Run, requestedAgentID, selectedAgentID string, launchArgs []string) {
	protocolMeta := map[string]interface{}{"requested_agent_id": requestedAgentID, "selected_agent_id": selectedAgentID}
	if resolved, err := agentlaunch.ResolveForAgent(s.endpointResolver, selectedAgentID, launchArgs...); err == nil && len(resolved.Metadata) > 0 {
		protocolMeta["agent_launch_policy"] = resolved.Metadata
	}
	_, _ = s.runStore.AppendEvent(runtrace.Event{
		RunID:        run.ID,
		Kind:         "routing.decision",
		Actor:        "matrix",
		Status:       runtrace.StatusCompleted,
		Protocol:     run.Protocol,
		DecisionID:   "decision-" + run.ID,
		ProtocolMeta: protocolMeta,
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
		writeRunResult(w, res, err, runWriteOptions{runID: exec.runID, status: runHTTPStatus(ctx, err, exec.emergencyTimeout)})
	}
}

// runHTTPStatus maps a routed turn's failure to the status the caller sees.
func runHTTPStatus(ctx context.Context, err error, emergencyTimeout time.Duration) int {
	switch {
	case isSetupRequired(err):
		return http.StatusConflict
	case runactivity.IsDeadline(ctx, err, emergencyTimeout), runactivity.IsTimeoutError(err):
		return http.StatusGatewayTimeout
	default:
		status, _ := providerfailure.HTTPStatus(err)
		return status
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
		return s.abortRun(runAbort{ctx: ctx, exec: exec, sessionCtx: sessionCtx, res: res, activity: activityState, err: err})
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
		return s.terminalResult(exec.runID, runExecutionResult{output: res.output, cleanup: cleanup, routeErr: err}), err
	}
	_, err = s.runStore.Complete(exec.runID, res.output, res.stopReason)
	s.untrackRunCancel(exec.runID)
	if err != nil {
		return s.terminalResult(exec.runID, runExecutionResult{output: res.output, cleanup: cleanup, routeErr: err}), err
	}
	return s.terminalResult(exec.runID, runExecutionResult{output: res.output, cleanup: cleanup}), nil
}

// terminalResult reports the terminal state a run actually reached. Reading it
// back from the record — rather than assuming that reaching this point means
// success — is what keeps a turn that produced nothing from being handed out as
// a completed result.
func (s *Server) terminalResult(runID string, res runExecutionResult) runExecutionResult {
	run, found, err := s.runStore.LoadRun(runID)
	if err != nil || !found {
		return res
	}
	// The delivery contract is settled here, where every terminal path
	// converges: once this returns, the run is over and the workspace is no
	// longer necessarily the one the work left behind.
	s.recordDeliveryVerdict(run)
	res.terminal = &run
	return res
}

// abortRun closes a run whose turn was aborted, and reports which terminal state
// the abort produced: a provider failure, an activity timeout, an emergency
// deadline or a cancellation. Which one it was is read from the typed errors the
// watchdogs return, never from the shape of the message.
func (s *Server) abortRun(abort runAbort) (runExecutionResult, error) {
	ctx, exec, sessionCtx, res := abort.ctx, abort.exec, abort.sessionCtx, abort.res
	activityState, err := abort.activity, abort.err
	postCtx, postCancel := postRunContext(ctx)
	defer postCancel()
	after := s.enrichRunFromSession(postCtx, sessionEnrichmentRequest{
		runID:       exec.runID,
		channelID:   exec.req.ChannelID,
		workspaceID: exec.req.WorkspaceID,
		before:      sessionCtx.before,
	})
	cleanup, _ := s.cleanupRunSessionContext(postCtx, exec, sessionCtx, after)
	// The aborting error is kept so a cancelled run still tells the caller why it
	// was cancelled: the run record names the stop reason, and this carries the
	// message that goes with it.
	abortErr := err
	switch {
	case runactivity.IsTimeout(activityState, err):
		abortErr = runactivity.Error(activityState)
		err = abortErr
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
	result := s.terminalResult(exec.runID, runExecutionResult{output: res.output, cleanup: cleanup, routeErr: err})
	result.abortErr = abortErr
	return result, err
}

// routeResult is what routing one turn produced. The stop reason travels beside
// the output because it is what the peer reported, in the peer's own words: the
// run record states it as reported, and an absent report stays absent instead of
// becoming an "end_turn" Matrix would be inventing.
type routeResult struct {
	output     string
	stopReason string
}

func (s *Server) route(ctx context.Context, exec runExecution, prepared sessionSnapshot, notifier middleware.ThoughtNotifier) (routeResult, error) {
	req := exec.req
	if richer, ok := s.router.(middleware.ConversationRequestRouter); ok {
		output, err := richer.RouteConversation(ctx, middleware.ConversationRequest{
			ChannelID:             req.ChannelID,
			AgentID:               exec.agentID,
			LogicalSessionID:      strings.TrimSpace(prepared.LogicalSessionID),
			WorkspaceID:           req.WorkspaceID,
			WorkspacePath:         req.WorkspacePath,
			Input:                 req.Input.String(),
			ModelID:               req.ModelID,
			FallbackModelID:       req.FallbackModelID,
			SidecarCapsules:       req.SidecarCapsules,
			AdditionalDirectories: req.AdditionalDirectories,
			AgentLaunchArgs:       req.agentLaunchArgs,
			Notifier:              notifier,
			NonInteractive:        true,
		})
		return routeResult{output: output}, err
	}
	output, err := s.router.Route(ctx, req.ChannelID, exec.agentID, sidecar.ProjectPrompt(req.Input.String(), req.SidecarCapsules), notifier)
	return routeResult{output: output}, err
}

// runNotCompletedError carries the terminal state a run actually reached into
// the HTTP handlers, which already know how to render an error.
type runNotCompletedError struct {
	runID  string
	status string
	detail string
}

func (e *runNotCompletedError) Error() string {
	if e == nil {
		return ""
	}
	if e.detail != "" {
		return e.detail
	}
	return "run " + e.runID + " reached terminal status " + e.status
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
	writeRunResult(w, res, err, runWriteOptions{runID: exec.runID, status: runHTTPStatus(ctx, err, exec.emergencyTimeout), stream: true})
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
