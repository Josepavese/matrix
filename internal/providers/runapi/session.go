package runapi

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/middleware"
)

const runCleanupTimeout = 30 * time.Second

type sessionSnapshot struct {
	LogicalSessionID string
	RemoteSessionID  string
	AgentID          string
	Protocol         string
	WorkspaceID      string
	Mode             string
	Status           string
}

type sessionEnrichmentRequest struct {
	runID       string
	channelID   string
	workspaceID string
	before      sessionSnapshot
}

func (s *Server) sessionSnapshot(ctx context.Context, channelID, workspaceID string) sessionSnapshot {
	result, err := s.router.HandleSessionActionTyped(ctx, middleware.SessionActionRequest{
		ChannelID:   channelID,
		Action:      "status",
		WorkspaceID: workspaceID,
	})
	if err != nil || result.Session == nil {
		return sessionSnapshot{}
	}
	return sessionSnapshotFromEntry(*result.Session)
}

func (s *Server) enrichRunFromSession(ctx context.Context, req sessionEnrichmentRequest) sessionSnapshot {
	after := s.sessionSnapshot(ctx, req.channelID, req.workspaceID)
	if after.LogicalSessionID == "" && after.RemoteSessionID == "" {
		return after
	}
	run, found, err := s.runStore.LoadRun(req.runID)
	if err != nil || !found {
		return after
	}
	run = enrichRun(run, after)
	if err := s.runStore.SaveRun(run); err != nil {
		slog.Warn("failed to enrich run session metadata", "error", err, "run_id", req.runID)
	}
	_, _ = s.runStore.AppendEvent(sessionEvent(req.runID, req.before, after))
	return after
}

func enrichRun(run runtrace.Run, after sessionSnapshot) runtrace.Run {
	if after.LogicalSessionID != "" {
		run.LogicalSessionID = after.LogicalSessionID
	}
	if after.RemoteSessionID != "" {
		run.RemoteSessionID = after.RemoteSessionID
	}
	if after.WorkspaceID != "" {
		run.WorkspaceID = after.WorkspaceID
	}
	if after.Protocol != "" {
		run.Protocol = after.Protocol
	}
	return run
}

func sessionEvent(runID string, before, after sessionSnapshot) runtrace.Event {
	kind := "session.resumed"
	if before.LogicalSessionID == "" || before.LogicalSessionID != after.LogicalSessionID {
		kind = "session.created"
	}
	return runtrace.Event{
		RunID:          runID,
		Kind:           kind,
		Actor:          "matrix",
		Status:         runtrace.StatusCompleted,
		Protocol:       after.Protocol,
		ProtocolMethod: "session.status",
		Metadata: map[string]interface{}{
			"logical_session_id": strings.TrimSpace(after.LogicalSessionID),
			"remote_session_id":  strings.TrimSpace(after.RemoteSessionID),
			"agent_id":           strings.TrimSpace(after.AgentID),
			"workspace_id":       strings.TrimSpace(after.WorkspaceID),
			"mode":               strings.TrimSpace(after.Mode),
			"status":             strings.TrimSpace(after.Status),
		},
	}
}

func sessionSnapshotFromEntry(entry middleware.SessionEntry) sessionSnapshot {
	return sessionSnapshot{
		LogicalSessionID: entry.LogicalSessionID,
		RemoteSessionID:  entry.RemoteSessionID,
		AgentID:          entry.AgentID,
		Protocol:         entry.ProtocolKind,
		WorkspaceID:      entry.WorkspaceID,
		Mode:             entry.Mode,
		Status:           entry.Status,
	}
}

func (s *Server) prepareSessionForRun(ctx context.Context, exec runExecution) error {
	if normalizeRunSessionPolicy(exec.req.SessionPolicy) != middleware.SessionPolicyNewEphemeralDeleteAfterRun {
		return nil
	}
	result, err := s.router.HandleSessionActionTyped(ctx, middleware.SessionActionRequest{
		ChannelID:     exec.req.ChannelID,
		Action:        "new",
		Target:        exec.agentID,
		WorkspaceID:   exec.req.WorkspaceID,
		WorkspacePath: exec.req.WorkspacePath,
		Ephemeral:     true,
		CleanupPolicy: cleanupPolicyForRun(exec.req),
	})
	if err != nil {
		return err
	}
	_, _ = s.runStore.AppendEvent(runtrace.Event{
		RunID:          exec.runID,
		Kind:           "session.policy.applied",
		Actor:          "matrix",
		Status:         runtrace.StatusCompleted,
		Protocol:       s.resolveProtocol(exec.agentID),
		ProtocolMethod: "session.new",
		Metadata: map[string]interface{}{
			"session_policy":     normalizeRunSessionPolicy(exec.req.SessionPolicy),
			"cleanup_policy":     cleanupPolicyForRun(exec.req),
			"logical_session_id": strings.TrimSpace(result.ActiveSessionID),
			"workspace_id":       strings.TrimSpace(exec.req.WorkspaceID),
			"workspace_path":     strings.TrimSpace(exec.req.WorkspacePath),
			"ephemeral":          true,
		},
	})
	return nil
}

func (s *Server) cleanupRunSession(ctx context.Context, exec runExecution, after sessionSnapshot) (*middleware.SessionCleanupResult, error) {
	target := strings.TrimSpace(after.LogicalSessionID)
	if target == "" {
		target = strings.TrimSpace(after.RemoteSessionID)
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
	if result.Cleanup != nil {
		s.appendCleanupEvent(exec.runID, exec.agentID, *result.Cleanup)
	}
	return result.Cleanup, err
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
		Metadata: map[string]interface{}{
			"logical_session_id":        cleanup.LogicalSessionID,
			"remote_session_id":         cleanup.RemoteSessionID,
			"agent_id":                  cleanup.AgentID,
			"protocol_kind":             cleanup.ProtocolKind,
			"cleanup_policy":            cleanup.CleanupPolicy,
			"clean":                     cleanup.Clean,
			"strong_cleanup":            cleanup.StrongCleanup,
			"cleanup_strength":          cleanup.CleanupStrength,
			"weak_cleanup_reason":       cleanup.WeakCleanupReason,
			"remote_delete_attempted":   cleanup.RemoteDeleteAttempted,
			"remote_deleted":            cleanup.RemoteDeleted,
			"remote_delete_unsupported": cleanup.RemoteDeleteUnsupported,
			"remote_close_attempted":    cleanup.RemoteCloseAttempted,
			"remote_closed":             cleanup.RemoteClosed,
			"remote_close_unsupported":  cleanup.RemoteCloseUnsupported,
			"remote_cancel_attempted":   cleanup.RemoteCancelAttempted,
			"remote_canceled":           cleanup.RemoteCanceled,
			"process_reap_attempted":    cleanup.ProcessReapAttempted,
			"process_reaped":            cleanup.ProcessReaped,
			"process_retained":          cleanup.ProcessRetained,
			"process_retention_allowed": cleanup.ProcessRetentionAllowed,
			"process_retention_reason":  cleanup.ProcessRetentionReason,
			"local_forgotten":           cleanup.LocalForgotten,
			"failure_code":              cleanup.FailureCode,
			"error":                     cleanup.Error,
		},
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
