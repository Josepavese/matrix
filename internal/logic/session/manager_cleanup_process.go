package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

func (m *Manager) reapAgentClientAfterLocalCleanup(ctx context.Context, req sessionCleanupExecution, result *middleware.SessionCleanupResult) {
	if !result.LocalForgotten || m.allowProcessRetention(req.Meta, result) {
		return
	}
	result.ProcessReapAttempted = true
	if reaper, ok := m.router.(middleware.AgentSessionClientReaper); ok && strings.TrimSpace(req.Meta.AgentSessionID) != "" {
		reaped, err := reaper.ReapAgentSessionClient(ctx, req.Meta.AgentID, req.Meta.AgentSessionID, req.Meta.WorkspacePath)
		recordAgentClientReapResult(reaped, err, result)
		return
	}
	reaper, ok := m.router.(middleware.AgentClientReaper)
	if !ok {
		result.ProcessRetained = true
		result.ProcessRetentionReason = "router does not expose agent client reaping"
		return
	}
	reaped, err := reaper.ReapAgentClient(ctx, req.Meta.AgentID, req.Meta.WorkspacePath)
	recordAgentClientReapResult(reaped, err, result)
}

func recordAgentClientReapResult(reaped bool, err error, result *middleware.SessionCleanupResult) {
	if err != nil {
		result.ProcessRetained = true
		result.ProcessRetentionReason = err.Error()
		result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "process_reap", err)
		return
	}
	if reaped {
		result.ProcessReaped = true
		return
	}
	result.ProcessAbsent = true
	result.ProcessAbsenceReason = sessioncleanup.NoMatchingCachedAgentClient
	result.ProcessRetentionReason = sessioncleanup.NoMatchingCachedAgentClient
}

func (m *Manager) allowProcessRetention(meta SessionMeta, result *middleware.SessionCleanupResult) bool {
	if meta.Ephemeral && strings.TrimSpace(meta.ParentSessionID) == "" {
		references, err := m.forkChildAgentClientReferences(meta)
		if err != nil {
			result.ProcessRetained = true
			result.ProcessRetentionReason = err.Error()
			result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "process_reap_refs", err)
			return true
		}
		if len(references) == 0 {
			return false
		}
		result.ProcessRetained = true
		result.ProcessRetentionAllowed = true
		result.ProcessRetentionReason = sessioncleanup.OtherLocalSessionsStillReferenceAgentClient
		return true
	}
	if strings.TrimSpace(meta.ParentSessionID) != "" || strings.TrimSpace(meta.ParentRemoteID) != "" {
		result.ProcessRetained = true
		result.ProcessRetentionAllowed = true
		result.ProcessRetentionReason = sessioncleanup.ForkChildUsesParentAgentClient
		return true
	}
	references, err := m.otherAgentClientReferences(meta)
	if err != nil {
		result.ProcessRetained = true
		result.ProcessRetentionReason = err.Error()
		result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "process_reap_refs", err)
		return true
	}
	if len(references) == 0 {
		return false
	}
	if cleanupHasRemoteTargetProof(result) {
		m.appendSharedAgentClientOwners(result, references)
		return true
	}
	result.ProcessRetained = true
	result.ProcessRetentionAllowed = true
	result.ProcessRetentionReason = sessioncleanup.OtherLocalSessionsStillReferenceAgentClient
	return true
}

func (m *Manager) otherAgentClientReferences(meta SessionMeta) ([]SessionMeta, error) {
	return m.agentClientReferences(meta, func(_ SessionMeta) bool {
		return true
	})
}

func (m *Manager) forkChildAgentClientReferences(meta SessionMeta) ([]SessionMeta, error) {
	parentID := strings.TrimSpace(meta.ID)
	parentRemoteID := strings.TrimSpace(meta.AgentSessionID)
	return m.agentClientReferences(meta, func(other SessionMeta) bool {
		if strings.TrimSpace(other.ParentSessionID) == parentID {
			return true
		}
		return parentRemoteID != "" && strings.TrimSpace(other.ParentRemoteID) == parentRemoteID
	})
}

func (m *Manager) agentClientReferences(meta SessionMeta, include func(SessionMeta) bool) ([]SessionMeta, error) {
	keys, err := m.storage.List("session.meta.")
	if err != nil {
		return nil, err
	}
	targetAgent := strings.TrimSpace(meta.AgentID)
	targetPath := cleanSessionWorkspacePath(meta.WorkspacePath)
	references := []SessionMeta{}
	for _, key := range keys {
		raw, err := m.storage.Get(key)
		if err != nil || len(raw) == 0 {
			continue
		}
		var other SessionMeta
		if err := json.Unmarshal(raw, &other); err != nil || other.ID == meta.ID {
			continue
		}
		if include != nil && !include(other) {
			continue
		}
		if strings.TrimSpace(other.AgentID) == targetAgent && cleanSessionWorkspacePath(other.WorkspacePath) == targetPath {
			references = append(references, other)
		}
	}
	return references, nil
}

func cleanSessionWorkspacePath(path string) string { return filepath.Clean(strings.TrimSpace(path)) }

func cleanupHasRemoteTargetProof(result *middleware.SessionCleanupResult) bool {
	return result.RemoteDeleted || result.RemoteClosed || result.RemoteCanceled
}

func (m *Manager) appendSharedAgentClientOwners(result *middleware.SessionCleanupResult, references []SessionMeta) {
	for _, ref := range references {
		related := middleware.SessionCleanupRelatedSession{
			LogicalSessionID: strings.TrimSpace(ref.ID),
			RemoteSessionID:  strings.TrimSpace(ref.AgentSessionID),
			AgentID:          strings.TrimSpace(ref.AgentID),
			ProtocolKind:     strings.TrimSpace(ref.ProtocolKind),
			WorkspaceID:      strings.TrimSpace(ref.WorkspaceID),
			WorkspacePath:    strings.TrimSpace(ref.WorkspacePath),
			ParentSessionID:  strings.TrimSpace(ref.ParentSessionID),
			ParentRemoteID:   strings.TrimSpace(ref.ParentRemoteID),
			Ephemeral:        ref.Ephemeral,
			CleanupPolicy:    strings.TrimSpace(ref.CleanupPolicy),
			Active:           m.sessionActiveInAnyChannel(ref.ID),
			Retained:         false,
			Reason:           sessioncleanup.ReasonSharedAgentClientOwner,
		}
		if cleanupRelatedSessionContains(result.RelatedSessions, related) {
			continue
		}
		result.RelatedSessions = append(result.RelatedSessions, related)
	}
}

func (m *Manager) sessionActiveInAnyChannel(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	keys, err := m.storage.List("session.channel.")
	if err != nil {
		return false
	}
	for _, key := range keys {
		raw, err := m.storage.Get(key)
		if err != nil || len(raw) == 0 {
			continue
		}
		var state ChannelState
		if err := json.Unmarshal(raw, &state); err != nil {
			continue
		}
		if strings.TrimSpace(state.ActiveSessionID) == sessionID {
			return true
		}
	}
	return false
}

func cleanupRelatedSessionContains(existing []middleware.SessionCleanupRelatedSession, candidate middleware.SessionCleanupRelatedSession) bool {
	for _, related := range existing {
		if strings.TrimSpace(related.LogicalSessionID) != "" && strings.TrimSpace(related.LogicalSessionID) == candidate.LogicalSessionID {
			return true
		}
		if strings.TrimSpace(related.RemoteSessionID) != "" && strings.TrimSpace(related.RemoteSessionID) == candidate.RemoteSessionID {
			return true
		}
	}
	return false
}
