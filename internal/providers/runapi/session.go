package runapi

import (
	"context"
	"log/slog"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/middleware"
)

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

func (s *Server) enrichRunFromSession(ctx context.Context, req sessionEnrichmentRequest) {
	after := s.sessionSnapshot(ctx, req.channelID, req.workspaceID)
	if after.LogicalSessionID == "" && after.RemoteSessionID == "" {
		return
	}
	run, found, err := s.runStore.LoadRun(req.runID)
	if err != nil || !found {
		return
	}
	run = enrichRun(run, after)
	if err := s.runStore.SaveRun(run); err != nil {
		slog.Warn("failed to enrich run session metadata", "error", err, "run_id", req.runID)
	}
	_, _ = s.runStore.AppendEvent(sessionEvent(req.runID, req.before, after))
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
