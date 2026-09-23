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

// reconcileOutcome is the result of one reconcile cycle. deniedRefs carries the
// refs reported by the attempt that was refused by the OS, so a failure can name
// the process that survived even when a later pass reports nothing.
type reconcileOutcome struct {
	result     middleware.SessionActionResult
	deniedRefs []middleware.AgentClientRef
	denied     bool
	resolved   bool
	err        error
}

func Apply(ctx context.Context, req Request) error {
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), req.Timeout)
	defer cancel()
	outcome := reconcileRunScopedClients(reconcileCtx, req)
	if outcome.denied {
		return applyDeniedOutcome(req, outcome)
	}
	if outcome.err != nil {
		return failRunCleanup(req.Cleanup, outcome.err)
	}
	if outcome.result.Unsupported || outcome.result.Reconcile == nil {
		return nil
	}
	reaped, retained := splitRunScopedRefs(req, outcome.result.Reconcile.Reaped, outcome.result.Reconcile.Retained)
	for _, ref := range reaped {
		req.Cleanup.RelatedSessions = append(req.Cleanup.RelatedSessions, relatedSession(ref, false))
	}
	failed := false
	for _, ref := range retained {
		if !markRetained(req.Cleanup, ref) {
			failed = true
		}
	}
	if failed {
		return sessioncleanup.FailureError(req.Cleanup, "")
	}
	return nil
}

// reconcileRunScopedClients invokes the reconcile action, retrying a transient
// OS refusal to terminate an agent client.
//
// Retries are bounded and classification-aware. The reconcile action evicts a
// client from the router cache before closing it, so a second pass cannot close
// the same process again. Retrying is therefore only sound when the refusal is
// attributable to retained cached shared clients: a no-op second pass there
// cannot hide a leak of the run's own child. A refusal attributable to a
// run-scoped child or wrapper is reported immediately, because a retry could
// report success while the child is still alive, which is exactly the
// false-success this change exists to prevent.
func reconcileRunScopedClients(ctx context.Context, req Request) reconcileOutcome {
	outcome := reconcileOutcome{}
	attempt := func() error {
		return reconcileOnce(ctx, req, &outcome)
	}
	if err := sessioncleanup.RetryTermination(ctx, attempt, func(err error) bool {
		return retryIsSound(req, err, outcome.deniedRefs)
	}); err != nil {
		outcome.err = err
		return outcome
	}
	outcome.resolved = outcome.denied
	return outcome
}

func reconcileOnce(ctx context.Context, req Request, outcome *reconcileOutcome) error {
	result, err := req.Router.HandleSessionActionTyped(ctx, middleware.SessionActionRequest{
		ChannelID: req.ChannelID,
		Action:    "reconcile",
	})
	if err != nil {
		if !outcome.denied {
			outcome.denied = true
			if result.Reconcile != nil {
				outcome.deniedRefs = inScopeRefs(req, result.Reconcile.Retained)
			}
		}
		outcome.err = err
		return err
	}
	outcome.err = nil
	if !outcome.denied {
		outcome.result = result
	}
	return nil
}

// retryIsSound reports whether another reconcile pass may be attempted. Only a
// denial attributable to cached shared clients is retryable; anything else fails
// closed so a run-scoped leak is never masked by a no-op second pass.
func retryIsSound(req Request, err error, deniedRefs []middleware.AgentClientRef) bool {
	if !sessioncleanup.IsTerminationAccessDenied(err) || len(deniedRefs) == 0 {
		return false
	}
	for _, ref := range deniedRefs {
		if classifyRetainedScope(ref, req.Cleanup) != sessioncleanup.ScopeCachedSharedClient {
			return false
		}
	}
	return true
}

// applyDeniedOutcome records a refused termination. A refusal on a cached shared
// client is a degraded cleanup that preserves the completed run; anything else is
// a real leak and fails the run. The OS denial text is preserved in the evidence
// so the operator sees what the kernel actually refused.
func applyDeniedOutcome(req Request, outcome reconcileOutcome) error {
	if outcome.resolved {
		// The retry confirmed the client was evicted and closed, so the run keeps
		// its original outcome and no retention evidence remains.
		return nil
	}
	shared := len(outcome.deniedRefs) > 0
	for _, ref := range outcome.deniedRefs {
		if !markRetained(req.Cleanup, ref) {
			shared = false
		}
	}
	markDenied(req.Cleanup, shared, outcome.err)
	if shared {
		return nil
	}
	return sessioncleanup.FailureError(req.Cleanup, "")
}

func inScopeRefs(req Request, refs []middleware.AgentClientRef) []middleware.AgentClientRef {
	inScope := make([]middleware.AgentClientRef, 0, len(refs))
	for _, ref := range refs {
		if matchesRunScope(req, ref) {
			inScope = append(inScope, ref)
		}
	}
	return inScope
}

func splitRunScopedRefs(req Request, reaped []middleware.AgentClientRef, retained []middleware.AgentClientRef) ([]middleware.AgentClientRef, []middleware.AgentClientRef) {
	return inScopeRefs(req, reaped), inScopeRefs(req, retained)
}

func relatedSession(ref middleware.AgentClientRef, retained bool) middleware.SessionCleanupRelatedSession {
	reason := sessioncleanup.ReasonRunUnreferencedAgentClientReaped
	if retained {
		reason = sessioncleanup.WarningRunRelatedSessionRetained
	}
	return middleware.SessionCleanupRelatedSession{
		LogicalSessionID: strings.TrimSpace(ref.LogicalSessionID),
		RemoteSessionID:  strings.TrimSpace(ref.RemoteSessionID),
		AgentID:          strings.TrimSpace(ref.AgentID),
		ProtocolKind:     strings.TrimSpace(ref.ProtocolKind),
		WorkspaceID:      strings.TrimSpace(ref.WorkspaceID),
		WorkspacePath:    strings.TrimSpace(ref.WorkspacePath),
		Retained:         retained,
		Reason:           reason,
	}
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

// classifyRetainedScope names WHICH process was retained. A ref that carries the
// run's own logical or remote session is the run-scoped ACP child; a ref with no
// session identity at all is the wrapper; anything else is a cached shared client
// retained on behalf of another session.
func classifyRetainedScope(ref middleware.AgentClientRef, cleanup *middleware.SessionCleanupResult) string {
	refLogical := strings.TrimSpace(ref.LogicalSessionID)
	refRemote := strings.TrimSpace(ref.RemoteSessionID)
	logical := ""
	remote := ""
	if cleanup != nil {
		logical = strings.TrimSpace(cleanup.LogicalSessionID)
		remote = strings.TrimSpace(cleanup.RemoteSessionID)
	}
	if refLogical == "" && refRemote == "" {
		return sessioncleanup.ScopeAgentWrapper
	}
	if logical != "" && refLogical == logical {
		return sessioncleanup.ScopeRunScopedAgentChild
	}
	if remote != "" && refRemote == remote {
		return sessioncleanup.ScopeRunScopedAgentChild
	}
	return sessioncleanup.ScopeCachedSharedClient
}

func retentionWarning(scope string) string {
	switch scope {
	case sessioncleanup.ScopeCachedSharedClient:
		return sessioncleanup.WarningCachedSharedClientRetained
	case sessioncleanup.ScopeAgentWrapper:
		return sessioncleanup.WarningRunRelatedSessionRetained
	default:
		return sessioncleanup.WarningRunScopedAgentChildRetained
	}
}

// markRetained records one retained ref and returns whether retention is
// acceptable. A cached shared client still serving a live local session is
// allowed retention: the governed transaction completed, so its cleanup is
// degraded rather than failed. A run-scoped child or wrapper is a real leak and
// fails the run.
func markRetained(cleanup *middleware.SessionCleanupResult, ref middleware.AgentClientRef) bool {
	if cleanup == nil {
		return true
	}
	scope := classifyRetainedScope(ref, cleanup)
	allowed := scope == sessioncleanup.ScopeCachedSharedClient
	cleanup.RelatedSessions = append(cleanup.RelatedSessions, relatedSession(ref, true))
	cleanup.ProcessRetained = true
	cleanup.ProcessRetentionAllowed = allowed
	cleanup.ProcessRetentionReason = retentionWarning(scope)
	cleanup.ProcessRetentionScope = scope
	cleanup.Warnings = sessioncleanup.AppendWarning(cleanup.Warnings, cleanup.ProcessRetentionReason)
	cleanup.StrongCleanup = false
	cleanup.WeakCleanupReason = sessioncleanup.WeakCleanupProcessRetained
	if allowed {
		// Cleanup stays operationally clean (remote state handled, no run-scoped
		// leak) while the recorded strength shows the degradation.
		cleanup.CleanupStrength = sessioncleanup.StrengthRetained
		return true
	}
	cleanup.Clean = false
	cleanup.CleanupStrength = sessioncleanup.StrengthFailed
	if cleanup.FailureCode == "" {
		cleanup.FailureCode = sessioncleanup.FailureRunRelatedSessionRetained
	}
	if cleanup.Error == "" {
		cleanup.Error = "run cleanup retained " + scope
	}
	return false
}

// markDenied records a refused termination the caller could not resolve by
// retrying. A shared client stays degraded; any other scope is a real leak.
func markDenied(cleanup *middleware.SessionCleanupResult, shared bool, err error) {
	if cleanup == nil {
		return
	}
	cleanup.ProcessRetained = true
	cleanup.WeakCleanupReason = sessioncleanup.WeakCleanupProcessRetained
	cleanup.StrongCleanup = false
	if shared {
		cleanup.CleanupStrength = sessioncleanup.StrengthRetained
		cleanup.ProcessRetentionAllowed = true
		cleanup.ProcessRetentionScope = sessioncleanup.ScopeCachedSharedClient
		cleanup.ProcessRetentionReason = sessioncleanup.WarningCachedSharedClientRetained
		cleanup.Warnings = sessioncleanup.AppendWarning(cleanup.Warnings, sessioncleanup.WarningCachedSharedClientRetained)
		cleanup.Error = sessioncleanup.AppendError(cleanup.Error, "agent_client_reconcile", err)
		return
	}
	cleanup.Clean = false
	cleanup.CleanupStrength = sessioncleanup.StrengthFailed
	cleanup.ProcessRetentionAllowed = false
	cleanup.ProcessRetentionScope = sessioncleanup.ScopeRunScopedAgentChild
	cleanup.ProcessRetentionReason = sessioncleanup.WarningRunAgentClientReconcileFailed
	cleanup.Warnings = sessioncleanup.AppendWarning(cleanup.Warnings, sessioncleanup.WarningRunAgentClientReconcileFailed)
	if cleanup.FailureCode == "" {
		cleanup.FailureCode = sessioncleanup.WarningRunAgentClientReconcileFailed
	}
	cleanup.Error = sessioncleanup.AppendError(cleanup.Error, "agent_client_reconcile_denied", err)
}

// failRunCleanup converts an unattributable reconcile error into failure
// evidence. The denial could not be tied to a retained ref, so it fails closed:
// the run fails rather than downgrading a possible run-scoped leak.
func failRunCleanup(cleanup *middleware.SessionCleanupResult, err error) error {
	if cleanup == nil {
		return nil
	}
	cleanup.ProcessRetained = true
	cleanup.ProcessRetentionAllowed = false
	cleanup.ProcessRetentionScope = sessioncleanup.ScopeRunScopedAgentChild
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
	return sessioncleanup.FailureError(cleanup, "")
}
