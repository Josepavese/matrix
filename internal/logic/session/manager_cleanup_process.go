package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/sessioncleanup"
	"github.com/jose/matrix-v2/internal/middleware"
)

func (m *Manager) reapAgentClientAfterLocalCleanup(ctx context.Context, req sessionCleanupExecution, result *middleware.SessionCleanupResult) {
	if !result.LocalForgotten || m.allowProcessRetention(req.Meta, result) {
		return
	}
	result.ProcessReapAttempted = true
	reaper, ok := m.router.(middleware.AgentClientReaper)
	if !ok {
		result.ProcessRetained = true
		result.ProcessRetentionReason = "router does not expose agent client reaping"
		return
	}
	reaped, err := reaper.ReapAgentClient(ctx, req.Meta.AgentID, req.Meta.WorkspacePath)
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
	result.ProcessRetentionReason = sessioncleanup.NoMatchingCachedAgentClient
}

func (m *Manager) allowProcessRetention(meta SessionMeta, result *middleware.SessionCleanupResult) bool {
	if meta.Ephemeral && strings.TrimSpace(meta.ParentSessionID) == "" {
		hasReferences, err := m.hasForkChildAgentClientReferences(meta)
		if err != nil {
			result.ProcessRetained = true
			result.ProcessRetentionReason = err.Error()
			result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "process_reap_refs", err)
			return true
		}
		if !hasReferences {
			return false
		}
		result.ProcessRetained = true
		result.ProcessRetentionAllowed = true
		result.ProcessRetentionReason = sessioncleanup.OtherLocalSessionsStillReferenceAgentClient
		return true
	}
	hasReferences, err := m.hasOtherAgentClientReferences(meta)
	if err != nil {
		result.ProcessRetained = true
		result.ProcessRetentionReason = err.Error()
		result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "process_reap_refs", err)
		return true
	}
	if !hasReferences {
		return false
	}
	result.ProcessRetained = true
	result.ProcessRetentionAllowed = true
	result.ProcessRetentionReason = sessioncleanup.OtherLocalSessionsStillReferenceAgentClient
	return true
}

func (m *Manager) hasOtherAgentClientReferences(meta SessionMeta) (bool, error) {
	return m.hasAgentClientReferences(meta, func(_ SessionMeta) bool {
		return true
	})
}

func (m *Manager) hasForkChildAgentClientReferences(meta SessionMeta) (bool, error) {
	parentID := strings.TrimSpace(meta.ID)
	parentRemoteID := strings.TrimSpace(meta.AgentSessionID)
	return m.hasAgentClientReferences(meta, func(other SessionMeta) bool {
		if strings.TrimSpace(other.ParentSessionID) == parentID {
			return true
		}
		return parentRemoteID != "" && strings.TrimSpace(other.ParentRemoteID) == parentRemoteID
	})
}

func (m *Manager) hasAgentClientReferences(meta SessionMeta, include func(SessionMeta) bool) (bool, error) {
	keys, err := m.storage.List("session.meta.")
	if err != nil {
		return false, err
	}
	targetAgent := strings.TrimSpace(meta.AgentID)
	targetPath := cleanSessionWorkspacePath(meta.WorkspacePath)
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
			return true, nil
		}
	}
	return false, nil
}

func cleanSessionWorkspacePath(path string) string {
	return filepath.Clean(strings.TrimSpace(path))
}
