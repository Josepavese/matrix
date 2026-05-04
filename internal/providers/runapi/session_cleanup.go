package runapi

import (
	"context"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/runreconcile"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

type runCleanupScope struct {
	exec       runExecution
	prepared   sessionSnapshot
	before     sessionSnapshot
	after      sessionSnapshot
	beforeList []sessionSnapshot
	afterList  []sessionSnapshot
}

type runSessionContext struct {
	before     sessionSnapshot
	beforeList []sessionSnapshot
	prepared   sessionSnapshot
}

func (s *Server) prepareRunSessionContext(ctx context.Context, exec runExecution) (runSessionContext, error) {
	before := s.sessionSnapshot(ctx, exec.req.ChannelID, exec.req.WorkspaceID)
	beforeList := s.sessionListSnapshot(ctx, exec.req.ChannelID, exec.req.WorkspaceID)
	prepared, err := s.prepareSessionForRun(ctx, exec)
	return runSessionContext{before: before, beforeList: beforeList, prepared: prepared}, err
}

func (s *Server) cleanupRunSessionContext(ctx context.Context, exec runExecution, state runSessionContext, after sessionSnapshot) (*middleware.SessionCleanupResult, error) {
	if !runRequiresCleanup(exec.req) {
		return nil, nil
	}
	afterList := s.sessionListSnapshot(ctx, exec.req.ChannelID, exec.req.WorkspaceID)
	return s.cleanupRunSessions(ctx, runCleanupScope{
		exec:       exec,
		prepared:   state.prepared,
		before:     state.before,
		after:      after,
		beforeList: state.beforeList,
		afterList:  afterList,
	})
}

func (s *Server) cleanupRunSessions(ctx context.Context, scope runCleanupScope) (*middleware.SessionCleanupResult, error) {
	target := cleanupTargetSnapshot(scope.prepared, scope.after)
	cleanup, err := s.cleanupRunSession(ctx, scope.exec, target)
	if cleanup == nil {
		return nil, err
	}
	if relatedErr := s.accountRunRelatedSessions(ctx, scope, cleanup, target); err == nil {
		err = relatedErr
	}
	if reconcileErr := runreconcile.Apply(ctx, runreconcile.Request{Timeout: runCleanupTimeout, Router: s.router, ChannelID: scope.exec.req.ChannelID, Cleanup: cleanup}); err == nil {
		err = reconcileErr
	}
	s.appendCleanupEvent(scope.exec.runID, scope.exec.agentID, *cleanup)
	return cleanup, err
}

func cleanupTargetSnapshot(prepared, after sessionSnapshot) sessionSnapshot {
	if strings.TrimSpace(prepared.LogicalSessionID) != "" || strings.TrimSpace(prepared.RemoteSessionID) != "" {
		return prepared
	}
	return after
}

func (s *Server) cleanupRunSession(ctx context.Context, exec runExecution, snapshot sessionSnapshot) (*middleware.SessionCleanupResult, error) {
	target := strings.TrimSpace(snapshot.LogicalSessionID)
	if target == "" {
		target = strings.TrimSpace(snapshot.RemoteSessionID)
	}
	if target == "" {
		return nil, nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	result, err := s.router.HandleSessionActionTyped(cleanupCtx, middleware.SessionActionRequest{
		ChannelID:        exec.req.ChannelID,
		Action:           "cleanup",
		Target:           target,
		CleanupPolicy:    cleanupPolicyForRun(exec.req),
		ForceForgetLocal: true,
	})
	if err != nil {
		return result.Cleanup, err
	}
	if result.Error != nil {
		return result.Cleanup, sessioncleanup.FailureError(result.Cleanup, result.Error.Message)
	}
	if result.Cleanup != nil && !result.Cleanup.Clean {
		return result.Cleanup, sessioncleanup.FailureError(result.Cleanup, "")
	}
	return result.Cleanup, nil
}

func (s *Server) accountRunRelatedSessions(ctx context.Context, scope runCleanupScope, cleanup *middleware.SessionCleanupResult, target sessionSnapshot) error {
	rec := newRunRelatedSessionRecorder(ctx, s, scope, cleanup)
	if !sameSessionSnapshot(target, scope.after) {
		rec.record(scope.after, true)
	}
	for _, snapshot := range scope.afterList {
		if sameSessionSnapshot(target, snapshot) || sameSessionSnapshot(scope.after, snapshot) {
			continue
		}
		if _, existed := rec.beforeSet[sessionSnapshotKey(snapshot)]; existed {
			continue
		}
		rec.record(snapshot, false)
	}
	return rec.cleanupErr
}

func (s *Server) appendCleanupEvent(runID, agentID string, cleanup middleware.SessionCleanupResult) {
	status := runtrace.StatusCompleted
	if !cleanup.Clean {
		status = runtrace.StatusFailed
	}
	_, _ = s.runStore.AppendEvent(runtrace.Event{
		RunID:          runID,
		Kind:           "session.cleanup",
		Actor:          "matrix",
		Status:         status,
		Protocol:       s.resolveProtocol(agentID),
		ProtocolMethod: "session.cleanup",
		Metadata:       sessioncleanup.Metadata(cleanup),
	})
}

func runRequiresCleanup(req runRequest) bool {
	return normalizeRunSessionPolicy(req.SessionPolicy) == middleware.SessionPolicyNewEphemeralDeleteAfterRun
}

func cleanupPolicyForRun(req runRequest) string {
	if strings.TrimSpace(req.CleanupPolicy) != "" {
		return req.CleanupPolicy
	}
	if normalizeRunSessionPolicy(req.SessionPolicy) == middleware.SessionPolicyNewEphemeralDeleteAfterRun {
		return middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal
	}
	return ""
}

func normalizeRunSessionPolicy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case middleware.SessionPolicyNewEphemeralDeleteAfterRun:
		return middleware.SessionPolicyNewEphemeralDeleteAfterRun
	default:
		return ""
	}
}
