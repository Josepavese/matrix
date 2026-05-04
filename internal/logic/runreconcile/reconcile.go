package runreconcile

import (
	"context"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

type Router interface {
	HandleSessionActionTyped(context.Context, middleware.SessionActionRequest) (middleware.SessionActionResult, error)
}

type Request struct {
	Timeout   time.Duration
	Router    Router
	ChannelID string
	Cleanup   *middleware.SessionCleanupResult
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
		req.Cleanup.RelatedSessions = append(req.Cleanup.RelatedSessions, middleware.SessionCleanupRelatedSession{
			AgentID:       strings.TrimSpace(ref.AgentID),
			WorkspacePath: strings.TrimSpace(ref.WorkspacePath),
			Reason:        sessioncleanup.ReasonRunUnreferencedAgentClientReaped,
		})
	}
	return nil
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
