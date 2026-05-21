package session

import (
	"context"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

func (m *Manager) finalizeForkChildCleanupProof(childMeta SessionMeta, cleanup *middleware.SessionCleanupResult) {
	if cleanup == nil || !isForkChildParentClientRetention(cleanup) {
		return
	}
	parent, found := m.loadForkParentMeta(childMeta)
	if found && forkParentOwnerProofAllowed(childMeta, parent, cleanup.CleanupPolicy) &&
		forkChildCleanupStronglyProven(cleanup) {
		appendForkParentRelatedSession(cleanup, forkParentRelatedSessionFromMeta(parent, false))
		markForkChildCleanupStrong(cleanup)
		return
	}
	markForkChildCleanupRetained(cleanup)
}

func (m *Manager) finalizeForkChildrenAfterParentCleanup(parentMeta SessionMeta, result *middleware.SessionCleanupResult) {
	if result == nil || len(result.ForkChildren) == 0 {
		return
	}
	parent := forkParentRelatedSessionFromMeta(parentMeta, false)
	for i := range result.ForkChildren {
		child := &result.ForkChildren[i]
		if !isForkChildParentClientRetention(child) {
			continue
		}
		appendForkParentRelatedSession(child, parent)
		if forkChildCleanupStronglyProven(child) {
			markForkChildCleanupStrong(child)
			continue
		}
		if result.ProcessReaped && child.LocalForgotten {
			child.ProcessReaped = true
			markForkChildCleanupStrong(child)
			continue
		}
		markForkChildCleanupRetained(child)
	}
}

func (m *Manager) failStandaloneForkChildCleanupIfRetained(childMeta SessionMeta, cleanup *middleware.SessionCleanupResult) {
	if cleanup == nil || !isForkChildParentClientRetention(cleanup) {
		return
	}
	appendForkParentRelatedSession(cleanup, m.forkParentRelatedSession(childMeta, true))
	markForkParentRelatedSessionRetained(cleanup)
	cleanup.Clean = false
	cleanup.StrongCleanup = false
	cleanup.CleanupStrength = sessioncleanup.StrengthFailed
	cleanup.WeakCleanupReason = sessioncleanup.WeakCleanupProcessRetained
	cleanup.Warnings = sessioncleanup.AppendWarning(cleanup.Warnings, sessioncleanup.WarningRunRelatedSessionRetained)
	cleanup.FailureCode = sessioncleanup.FailureRunRelatedSessionRetained
	if cleanup.Error == "" {
		cleanup.Error = "standalone fork child cleanup retained its parent agent client"
	}
}

func (m *Manager) cleanupRunOwnedForkParentOwnerFromStandaloneChild(ctx context.Context, req sessionCleanupExecution, policy string, cleanup *middleware.SessionCleanupResult) {
	parent, found := m.loadForkParentMeta(req.Meta)
	if !found {
		return
	}
	if !forkChildParentOwnerProofCandidate(req, cleanup) {
		return
	}
	if !forkParentOwnerProofAllowed(req.Meta, parent, policy) {
		return
	}
	appendForkParentRelatedSession(cleanup, forkParentRelatedSessionFromMeta(parent, false))
	if forkChildCleanupStronglyProven(cleanup) {
		markForkChildCleanupStrong(cleanup)
		return
	}
	if req.SuppressForkParentOwnerCleanup {
		appendForkParentRelatedSession(cleanup, forkParentRelatedSessionFromMeta(parent, true))
		return
	}
	if !forkParentOwnerCleanupAllowed(req.Meta, parent, policy) {
		appendForkParentRelatedSession(cleanup, forkParentRelatedSessionFromMeta(parent, true))
		return
	}
	parentCleanup := m.cleanupSessionMirrorAndRemote(ctx, sessionCleanupExecution{
		ChannelID:                      req.ChannelID,
		Meta:                           parent,
		CleanupPolicy:                  policy,
		ForceForgetLocal:               true,
		SuppressForkParentOwnerCleanup: true,
	})
	if !parentCleanup.Clean || parentCleanup.ProcessRetained {
		appendForkParentRelatedSession(cleanup, forkParentRelatedSessionFromMeta(parent, true))
		return
	}
	cleanup.ProcessReapAttempted = cleanup.ProcessReapAttempted || parentCleanup.ProcessReapAttempted
	cleanup.ProcessReaped = cleanup.ProcessReaped || parentCleanup.ProcessReaped
	markForkChildCleanupStrong(cleanup)
}

func forkChildParentOwnerProofCandidate(req sessionCleanupExecution, cleanup *middleware.SessionCleanupResult) bool {
	return cleanup != nil &&
		isForkChildParentClientRetention(cleanup) &&
		req.ForceForgetLocal
}

func forkParentOwnerProofAllowed(child, parent SessionMeta, policy string) bool {
	if !sameForkParent(child, parent) {
		return false
	}
	if sessioncleanup.NormalizePolicy(policy) == middleware.SessionCleanupPolicyForgetLocal {
		return false
	}
	return strings.TrimSpace(child.AgentID) == strings.TrimSpace(parent.AgentID) &&
		cleanSessionWorkspacePath(child.WorkspacePath) == cleanSessionWorkspacePath(parent.WorkspacePath)
}

func forkParentOwnerCleanupAllowed(child, parent SessionMeta, policy string) bool {
	if strings.TrimSpace(parent.OwnerRunID) == "" {
		return false
	}
	if !childCleanupOwned(child) || strings.TrimSpace(parent.CleanupPolicy) == "" {
		return false
	}
	return forkParentOwnerProofAllowed(child, parent, policy)
}

func childCleanupOwned(child SessionMeta) bool {
	return child.Ephemeral || strings.TrimSpace(child.CleanupPolicy) != ""
}

func sameForkParent(child, parent SessionMeta) bool {
	parentID := strings.TrimSpace(parent.ID)
	if parentID != "" && strings.TrimSpace(child.ParentSessionID) == parentID {
		return true
	}
	parentRemoteID := strings.TrimSpace(parent.AgentSessionID)
	return parentRemoteID != "" && strings.TrimSpace(child.ParentRemoteID) == parentRemoteID
}

func isForkChildParentClientRetention(cleanup *middleware.SessionCleanupResult) bool {
	return cleanup.ProcessRetained &&
		strings.TrimSpace(cleanup.ProcessRetentionReason) == sessioncleanup.ForkChildUsesParentAgentClient
}

func forkChildRemoteCleanupStrong(cleanup *middleware.SessionCleanupResult) bool {
	return cleanup.RemoteDeleted || cleanup.RemoteClosed || cleanup.RemoteCanceled
}

func forkChildProviderCleanupStrong(cleanup *middleware.SessionCleanupResult) bool {
	return cleanup != nil && (forkChildRemoteCleanupStrong(cleanup) ||
		sessioncleanup.HasWarning(cleanup.Warnings, sessioncleanup.WarningRemoteLifecycleSkippedNoReusableClient))
}

func forkChildCleanupStronglyProven(cleanup *middleware.SessionCleanupResult) bool {
	return cleanup != nil && cleanup.LocalForgotten && forkChildProviderCleanupStrong(cleanup)
}

func markForkChildCleanupStrong(cleanup *middleware.SessionCleanupResult) {
	cleanup.ProcessRetained = false
	cleanup.ProcessRetentionAllowed = false
	cleanup.ProcessRetentionReason = ""
	cleanup.Clean = true
	cleanup.StrongCleanup = true
	cleanup.CleanupStrength = sessioncleanup.StrengthStrong
	cleanup.WeakCleanupReason = ""
	cleanup.FailureCode = ""
	cleanup.Error = ""
}

func markForkChildCleanupRetained(cleanup *middleware.SessionCleanupResult) {
	markForkParentRelatedSessionRetained(cleanup)
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
		cleanup.Error = "fork child cleanup retained a related parent agent client"
	}
}

func (m *Manager) forkParentRelatedSession(childMeta SessionMeta, retained bool) middleware.SessionCleanupRelatedSession {
	parent, found := m.loadForkParentMeta(childMeta)
	if found {
		return forkParentRelatedSessionFromMeta(parent, retained)
	}
	return middleware.SessionCleanupRelatedSession{
		LogicalSessionID: strings.TrimSpace(childMeta.ParentSessionID),
		RemoteSessionID:  strings.TrimSpace(childMeta.ParentRemoteID),
		AgentID:          strings.TrimSpace(childMeta.AgentID),
		ProtocolKind:     strings.TrimSpace(childMeta.ProtocolKind),
		WorkspaceID:      strings.TrimSpace(childMeta.WorkspaceID),
		WorkspacePath:    strings.TrimSpace(childMeta.WorkspacePath),
		Retained:         retained,
		Reason:           sessioncleanup.ReasonForkParentAgentClientOwner,
	}
}

func (m *Manager) loadForkParentMeta(childMeta SessionMeta) (SessionMeta, bool) {
	parentID := strings.TrimSpace(childMeta.ParentSessionID)
	if parentID == "" {
		return SessionMeta{}, false
	}
	parent, found, err := m.loadSessionMeta(parentID)
	if err != nil || !found {
		return SessionMeta{}, false
	}
	return parent, true
}

func forkParentRelatedSessionFromMeta(parent SessionMeta, retained bool) middleware.SessionCleanupRelatedSession {
	return middleware.SessionCleanupRelatedSession{
		LogicalSessionID: strings.TrimSpace(parent.ID),
		RemoteSessionID:  strings.TrimSpace(parent.AgentSessionID),
		AgentID:          strings.TrimSpace(parent.AgentID),
		ProtocolKind:     strings.TrimSpace(parent.ProtocolKind),
		WorkspaceID:      strings.TrimSpace(parent.WorkspaceID),
		WorkspacePath:    strings.TrimSpace(parent.WorkspacePath),
		Ephemeral:        parent.Ephemeral,
		CleanupPolicy:    strings.TrimSpace(parent.CleanupPolicy),
		Retained:         retained,
		Reason:           sessioncleanup.ReasonForkParentAgentClientOwner,
	}
}

func appendForkParentRelatedSession(cleanup *middleware.SessionCleanupResult, related middleware.SessionCleanupRelatedSession) {
	if cleanup == nil {
		return
	}
	for i := range cleanup.RelatedSessions {
		existing := cleanup.RelatedSessions[i]
		if strings.TrimSpace(existing.LogicalSessionID) == strings.TrimSpace(related.LogicalSessionID) &&
			strings.TrimSpace(existing.RemoteSessionID) == strings.TrimSpace(related.RemoteSessionID) {
			cleanup.RelatedSessions[i] = related
			return
		}
	}
	cleanup.RelatedSessions = append(cleanup.RelatedSessions, related)
}

func markForkParentRelatedSessionRetained(cleanup *middleware.SessionCleanupResult) {
	for i := range cleanup.RelatedSessions {
		if cleanup.RelatedSessions[i].Reason == sessioncleanup.ReasonForkParentAgentClientOwner {
			cleanup.RelatedSessions[i].Retained = true
			cleanup.RelatedSessions[i].Reason = sessioncleanup.WarningRunRelatedSessionRetained
			return
		}
	}
}
