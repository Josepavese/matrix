package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

type sessionCleanupRequest struct {
	ChannelID, Lang, Target, Action, CleanupPolicy   string
	ForceForgetLocal, SuppressForkParentOwnerCleanup bool
}

type sessionCleanupExecution struct {
	ChannelID, CleanupPolicy                         string
	Meta                                             SessionMeta
	ForceForgetLocal, SuppressForkParentOwnerCleanup bool
}

func (m *Manager) handleSessionDeleteTyped(ctx context.Context, req sessionCleanupRequest) (middleware.SessionActionResult, error) {
	req.Action = "delete"
	return m.cleanupSessionTyped(ctx, req)
}

func (m *Manager) handleSessionCleanupTyped(ctx context.Context, req sessionCleanupRequest) (middleware.SessionActionResult, error) {
	req.Action = "cleanup"
	return m.cleanupSessionTyped(ctx, req)
}

func (m *Manager) cleanupSessionTyped(ctx context.Context, req sessionCleanupRequest) (middleware.SessionActionResult, error) {
	targetID := strings.TrimSpace(req.Target)
	if targetID == "" {
		state, err := m.getChannelState(req.ChannelID)
		if err != nil {
			return middleware.SessionActionResult{}, err
		}
		targetID = state.ActiveSessionID
	}
	if targetID == "" {
		return middleware.SessionActionResult{Action: req.Action, Message: m.wizard.GetString(req.Lang, "session_history_empty")}, nil
	}
	state, err := m.getChannelState(req.ChannelID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	metas, err := m.loadSessionMetas(state.History)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if resolved := resolveSessionTarget(targetID, state, metas); resolved != "" {
		targetID = resolved
	}
	meta, found, err := m.loadSessionMeta(targetID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if !found {
		return m.cleanupMissingLocalSession(ctx, req.ChannelID, targetID)
	}
	cleanup := m.cleanupSessionMirrorAndRemote(ctx, sessionCleanupExecution{
		ChannelID:                      req.ChannelID,
		Meta:                           meta,
		CleanupPolicy:                  req.CleanupPolicy,
		ForceForgetLocal:               req.ForceForgetLocal,
		SuppressForkParentOwnerCleanup: req.SuppressForkParentOwnerCleanup,
	})
	result := middleware.SessionActionResult{
		Action:  req.Action,
		Message: sessioncleanup.Message(req.Action, cleanup),
		Session: m.toSessionEntry(meta, false),
		Cleanup: &cleanup,
	}
	if cleanup.Error != "" || !cleanup.Clean {
		result.Error = sessioncleanup.ActionError(cleanup, targetID)
		sessioncleanup.LogTypedFailure(req.Action, targetID, cleanup)
	}
	return result, nil
}

func (m *Manager) cleanupMissingLocalSession(ctx context.Context, channelID, targetID string) (middleware.SessionActionResult, error) {
	deleted, handled, err := m.tryDeleteRemoteSession(ctx, channelID, targetID)
	if handled {
		if err != nil {
			return middleware.SessionActionResult{}, err
		}
		return deleted, nil
	}
	return middleware.SessionActionResult{}, fmt.Errorf("session %s not found", targetID)
}

func (m *Manager) deleteRemoteSession(ctx context.Context, meta SessionMeta) error {
	if strings.TrimSpace(meta.AgentSessionID) == "" {
		return nil
	}
	if controller, ok := m.router.(middleware.AgentWorkspaceSessionController); ok && strings.TrimSpace(meta.WorkspacePath) != "" {
		return controller.DeleteAgentSessionForWorkspace(ctx, meta.AgentID, meta.AgentSessionID, meta.WorkspacePath)
	}
	controller, ok := m.router.(middleware.AgentSessionController)
	if !ok {
		return fmt.Errorf("agent router does not expose remote session control")
	}
	return controller.DeleteAgentSession(ctx, meta.AgentID, meta.AgentSessionID)
}

func (m *Manager) closeRemoteSession(ctx context.Context, meta SessionMeta) error {
	if strings.TrimSpace(meta.AgentSessionID) == "" {
		return nil
	}
	if controller, ok := m.router.(middleware.AgentWorkspaceSessionController); ok && strings.TrimSpace(meta.WorkspacePath) != "" {
		return controller.CloseAgentSessionForWorkspace(ctx, meta.AgentID, meta.AgentSessionID, meta.WorkspacePath)
	}
	controller, ok := m.router.(middleware.AgentSessionController)
	if !ok {
		return fmt.Errorf("agent router does not expose remote session close")
	}
	return controller.CloseAgentSession(ctx, meta.AgentID, meta.AgentSessionID)
}

func (m *Manager) cleanupSessionMirrorAndRemote(ctx context.Context, req sessionCleanupExecution) middleware.SessionCleanupResult {
	policySource := strings.TrimSpace(req.CleanupPolicy)
	if policySource == "" {
		policySource = req.Meta.CleanupPolicy
	}
	policy := sessioncleanup.NormalizePolicy(policySource)
	result := middleware.SessionCleanupResult{
		LogicalSessionID: req.Meta.ID,
		RemoteSessionID:  req.Meta.AgentSessionID,
		AgentID:          req.Meta.AgentID,
		ProtocolKind:     req.Meta.ProtocolKind,
		CleanupPolicy:    policy,
	}
	m.cleanupForkChildren(ctx, req, policy, &result)
	m.cleanupRemoteSession(ctx, req.Meta, policy, &result)
	m.cleanupLocalMirror(req, result.RemoteDeleted, &result)
	m.reapAgentClientAfterLocalCleanup(ctx, req, &result)
	m.finalizeForkChildrenAfterParentCleanup(req.Meta, &result)
	m.markForkChildCleanupErrors(&result)
	finalizeCleanupResult(req.Meta, &result)
	m.cleanupRunOwnedForkParentOwnerFromStandaloneChild(ctx, req, policy, &result)
	m.failStandaloneForkChildCleanupIfRetained(req.Meta, &result)
	m.recordWorkspaceEvent(req.Meta, "session.cleanup", req.ChannelID, "Cleaned up session", "session-cleanup", sessioncleanup.Metadata(result))
	return result
}

func (m *Manager) cleanupRemoteSession(ctx context.Context, meta SessionMeta, policy string, result *middleware.SessionCleanupResult) {
	if shouldSkipRemoteCleanup(meta, policy) {
		return
	}
	m.tryRemoteDelete(ctx, meta, result)
	if result.RemoteDeleted || remoteLifecycleSkipped(result) || !sessioncleanup.AllowsCancelFallback(policy) {
		return
	}
	if result.RemoteDeleteUnsupported && m.tryRemoteClose(ctx, meta, result) {
		return
	}
	m.tryRemoteCancel(ctx, meta, result)
}

func shouldSkipRemoteCleanup(meta SessionMeta, policy string) bool {
	return strings.TrimSpace(meta.AgentSessionID) == "" || policy == middleware.SessionCleanupPolicyForgetLocal
}

func (m *Manager) tryRemoteDelete(ctx context.Context, meta SessionMeta, result *middleware.SessionCleanupResult) {
	result.RemoteDeleteAttempted = true
	err := m.deleteRemoteSession(ctx, meta)
	if err == nil {
		result.RemoteDeleted = true
		return
	}
	if appendNoReusableWarning(err, result) {
		return
	}
	result.RemoteDeleteUnsupported = sessioncleanup.IsRemoteDeleteUnsupported(err)
	result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "remote_delete", err)
}

func (m *Manager) tryRemoteClose(ctx context.Context, meta SessionMeta, result *middleware.SessionCleanupResult) bool {
	result.RemoteCloseAttempted = true
	err := m.closeRemoteSession(ctx, meta)
	if err == nil {
		result.RemoteClosed = true
		return true
	}
	if appendNoReusableWarning(err, result) {
		return true
	}
	result.RemoteCloseUnsupported = sessioncleanup.IsRemoteCloseUnsupported(err)
	result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "remote_close", err)
	return false
}

func (m *Manager) tryRemoteCancel(ctx context.Context, meta SessionMeta, result *middleware.SessionCleanupResult) {
	result.RemoteCancelAttempted = true
	if err := m.cancelRemoteSession(ctx, meta); err != nil {
		if appendNoReusableWarning(err, result) {
			return
		}
		result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "remote_cancel", err)
		return
	}
	result.RemoteCanceled = true
}

func appendNoReusableWarning(err error, result *middleware.SessionCleanupResult) bool {
	if !sessioncleanup.IsNoReusableCachedAgentClient(err) {
		return false
	}
	result.Warnings = sessioncleanup.AppendWarning(result.Warnings, sessioncleanup.WarningRemoteLifecycleSkippedNoReusableClient)
	return true
}

func remoteLifecycleSkipped(result *middleware.SessionCleanupResult) bool {
	return sessioncleanup.HasWarning(result.Warnings, sessioncleanup.WarningRemoteLifecycleSkippedNoReusableClient)
}

func (m *Manager) cleanupLocalMirror(req sessionCleanupExecution, remoteDeleted bool, result *middleware.SessionCleanupResult) {
	if sessioncleanup.ShouldForgetLocalMirror(result.CleanupPolicy, req.ForceForgetLocal, remoteDeleted) || strings.TrimSpace(req.Meta.AgentSessionID) == "" {
		if err := m.removeSessionMirror(req.ChannelID, req.Meta.ID); err != nil {
			result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "local_forget", err)
		} else {
			result.LocalForgotten = true
		}
		return
	}
	if result.RemoteCanceled {
		meta := req.Meta
		meta.RemoteStatus = "canceled"
		meta.LastSyncedAt = time.Now().UTC()
		if err := m.saveSessionMeta(meta); err != nil {
			result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "local_status", err)
		}
	}
}

func (m *Manager) cancelRemoteSession(ctx context.Context, meta SessionMeta) error {
	if strings.TrimSpace(meta.AgentSessionID) == "" {
		return nil
	}
	if controller, ok := m.router.(middleware.AgentWorkspaceSessionController); ok && strings.TrimSpace(meta.WorkspacePath) != "" {
		return controller.CancelAgentSessionForWorkspace(ctx, meta.AgentID, meta.AgentSessionID, meta.WorkspacePath)
	}
	controller, ok := m.router.(middleware.AgentSessionController)
	if !ok {
		return fmt.Errorf("agent router does not expose remote session control")
	}
	return controller.CancelAgentSession(ctx, meta.AgentID, meta.AgentSessionID)
}
