package runreconcile

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

type Router interface {
	HandleSessionActionTyped(context.Context, middleware.SessionActionRequest) (middleware.SessionActionResult, error)
}

type Request struct {
	Timeout       time.Duration
	Router        Router
	ChannelID     string
	AgentID       string
	WorkspacePath string
	Cleanup       *middleware.SessionCleanupResult
}

func Apply(ctx context.Context, req Request) error {
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), req.Timeout)
	defer cancel()
	result, err := req.Router.HandleSessionActionTyped(reconcileCtx, middleware.SessionActionRequest{
		ChannelID: req.ChannelID,
		Action:    "reconcile",
	})
	if err != nil {
		markFailed(req.Cleanup, err)
		return sessioncleanup.FailureError(req.Cleanup, "")
	}
	if result.Unsupported || result.Reconcile == nil {
		return nil
	}
	for _, ref := range result.Reconcile.Reaped {
		if !matchesRunScope(req, ref) {
			continue
		}
		req.Cleanup.RelatedSessions = append(req.Cleanup.RelatedSessions, middleware.SessionCleanupRelatedSession{
			LogicalSessionID: strings.TrimSpace(ref.LogicalSessionID),
			RemoteSessionID:  strings.TrimSpace(ref.RemoteSessionID),
			AgentID:          strings.TrimSpace(ref.AgentID),
			ProtocolKind:     strings.TrimSpace(ref.ProtocolKind),
			WorkspaceID:      strings.TrimSpace(ref.WorkspaceID),
			WorkspacePath:    strings.TrimSpace(ref.WorkspacePath),
			Reason:           sessioncleanup.ReasonRunUnreferencedAgentClientReaped,
		})
	}
	retained := 0
	for _, ref := range result.Reconcile.Retained {
		if !matchesRunScope(req, ref) {
			continue
		}
		markRetained(req.Cleanup, ref)
		retained++
	}
	if retained > 0 {
		return sessioncleanup.FailureError(req.Cleanup, "")
	}
	return nil
}

func matchesRunScope(req Request, ref middleware.AgentClientRef) bool {
	agentID := strings.TrimSpace(req.AgentID)
	if agentID != "" && strings.TrimSpace(ref.AgentID) != agentID {
		return false
	}
	workspacePath := strings.TrimSpace(req.WorkspacePath)
	if workspacePath == "" {
		return true
	}
	refPath := strings.TrimSpace(ref.WorkspacePath)
	if refPath == "" {
		return false
	}
	return filepath.Clean(refPath) == filepath.Clean(workspacePath)
}

func markRetained(cleanup *middleware.SessionCleanupResult, ref middleware.AgentClientRef) {
	if cleanup == nil {
		return
	}
	cleanup.RelatedSessions = append(cleanup.RelatedSessions, middleware.SessionCleanupRelatedSession{
		LogicalSessionID: strings.TrimSpace(ref.LogicalSessionID),
		RemoteSessionID:  strings.TrimSpace(ref.RemoteSessionID),
		AgentID:          strings.TrimSpace(ref.AgentID),
		ProtocolKind:     strings.TrimSpace(ref.ProtocolKind),
		WorkspaceID:      strings.TrimSpace(ref.WorkspaceID),
		WorkspacePath:    strings.TrimSpace(ref.WorkspacePath),
		Retained:         true,
		Reason:           sessioncleanup.WarningRunRelatedSessionRetained,
	})
	cleanup.ProcessRetained = true
	cleanup.ProcessRetentionAllowed = true
	cleanup.ProcessRetentionReason = sessioncleanup.WarningRunRelatedSessionRetained
	cleanup.Clean = false
	cleanup.StrongCleanup = false
	cleanup.CleanupStrength = sessioncleanup.StrengthFailed
	cleanup.WeakCleanupReason = sessioncleanup.WeakCleanupProcessRetained
	cleanup.Warnings = sessioncleanup.AppendWarning(cleanup.Warnings, sessioncleanup.WarningRunRelatedSessionRetained)
	if cleanup.FailureCode == "" {
		cleanup.FailureCode = sessioncleanup.FailureRunRelatedSessionRetained
	}
	if cleanup.Error == "" {
		cleanup.Error = "run cleanup retained a related agent client"
	}
}

func markFailed(cleanup *middleware.SessionCleanupResult, err error) {
	cleanup.ProcessRetained = true
	cleanup.ProcessRetentionReason = sessioncleanup.WarningRunAgentClientReconcileFailed
	cleanup.Clean = false
	cleanup.StrongCleanup = false
	cleanup.CleanupStrength = sessioncleanup.StrengthFailed
	cleanup.WeakCleanupReason = sessioncleanup.WeakCleanupProcessRetained
	cleanup.Warnings = sessioncleanup.AppendWarning(cleanup.Warnings, sessioncleanup.WarningRunAgentClientReconcileFailed)
	if cleanup.FailureCode == "" {
		cleanup.FailureCode = sessioncleanup.WarningRunAgentClientReconcileFailed
	}
	cleanup.Error = sessioncleanup.AppendError(cleanup.Error, "agent_client_reconcile", err)
}
