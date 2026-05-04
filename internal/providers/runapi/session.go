package runapi

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

const runCleanupTimeout = 30 * time.Second

type sessionSnapshot struct {
	LogicalSessionID string
	RemoteSessionID  string
	AgentID          string
	Protocol         string
	WorkspaceID      string
	WorkspacePath    string
	Mode             string
	Status           string
	Active           bool
	Ephemeral        bool
	CleanupPolicy    string
	ParentSessionID  string
	ParentRemoteID   string
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

func (s *Server) sessionListSnapshot(ctx context.Context, channelID, workspaceID string) []sessionSnapshot {
	result, err := s.router.HandleSessionActionTyped(ctx, middleware.SessionActionRequest{
		ChannelID:   channelID,
		Action:      "list",
		WorkspaceID: workspaceID,
		LocalOnly:   true,
	})
	if err != nil || len(result.Sessions) == 0 {
		return nil
	}
	snapshots := make([]sessionSnapshot, 0, len(result.Sessions))
	for _, entry := range result.Sessions {
		snapshots = append(snapshots, sessionSnapshotFromEntry(entry))
	}
	return snapshots
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
			"workspace_path":     strings.TrimSpace(after.WorkspacePath),
			"mode":               strings.TrimSpace(after.Mode),
			"status":             strings.TrimSpace(after.Status),
			"ephemeral":          after.Ephemeral,
			"cleanup_policy":     strings.TrimSpace(after.CleanupPolicy),
			"parent_session_id":  strings.TrimSpace(after.ParentSessionID),
			"parent_remote_id":   strings.TrimSpace(after.ParentRemoteID),
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
		WorkspacePath:    entry.WorkspacePath,
		Mode:             entry.Mode,
		Status:           entry.Status,
		Active:           entry.Active,
		Ephemeral:        entry.Ephemeral,
		CleanupPolicy:    entry.CleanupPolicy,
		ParentSessionID:  entry.ParentSessionID,
		ParentRemoteID:   entry.ParentRemoteID,
	}
}

func (s *Server) prepareSessionForRun(ctx context.Context, exec runExecution) (sessionSnapshot, error) {
	if normalizeRunSessionPolicy(exec.req.SessionPolicy) != middleware.SessionPolicyNewEphemeralDeleteAfterRun {
		return sessionSnapshot{}, nil
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
		return sessionSnapshot{}, err
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
	return preparedSessionSnapshot(result, exec), nil
}

func preparedSessionSnapshot(result middleware.SessionActionResult, exec runExecution) sessionSnapshot {
	if result.Session != nil {
		return sessionSnapshotFromEntry(*result.Session)
	}
	return sessionSnapshot{
		LogicalSessionID: strings.TrimSpace(result.ActiveSessionID),
		AgentID:          strings.TrimSpace(exec.agentID),
		WorkspaceID:      strings.TrimSpace(exec.req.WorkspaceID),
	}
}
