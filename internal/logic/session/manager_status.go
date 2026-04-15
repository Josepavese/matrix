package session

import (
	"fmt"
	"strings"
)

func (m *Manager) handleStatusCommand(channelID string) (string, error) {
	lang := m.wizard.GetLanguage(channelID)
	state, _ := m.getChannelState(channelID)
	if strings.TrimSpace(state.ActiveSessionID) == "" && strings.TrimSpace(state.PreferredWorkspaceID) == "" {
		return m.wizard.GetString(lang, "status_empty"), nil
	}

	var (
		workspaceID   string
		workspacePath string
		agentID       string
		mode          string
		remoteStatus  string
		title         string
		sessionID     string
		handoffLine   string
	)

	if strings.TrimSpace(state.ActiveSessionID) != "" {
		if meta, found, _ := m.loadSessionMeta(state.ActiveSessionID); found {
			sessionID = meta.ID
			workspaceID = meta.WorkspaceID
			workspacePath = meta.WorkspacePath
			agentID = meta.AgentID
			mode = normalizeMode(meta.Mode)
			remoteStatus = meta.RemoteStatus
			title = meta.RemoteTitle
			if meta.LastHandoff != nil && meta.LastHandoff.FromAgentID != "" {
				handoffLine = fmt.Sprintf("\nHandoff: %s -> %s", meta.LastHandoff.FromAgentID, valueOrDash(meta.LastHandoff.ToAgentID))
			}
		}
	}

	if workspaceID == "" {
		workspaceID = state.PreferredWorkspaceID
	}
	if workspaceID == "" {
		workspaceID = "-"
	}
	if workspacePath == "" {
		workspacePath = "-"
	}
	if agentID == "" {
		agentID = m.defaultAgent
	}
	if mode == "" {
		mode = modeImplementation
	}
	if remoteStatus == "" {
		remoteStatus = "active"
	}
	if sessionID == "" {
		sessionID = "-"
	}
	if title == "" {
		title = "-"
	}

	shortSessionID := sessionID
	if sessionID != "-" && len(sessionID) > 8 {
		shortSessionID = sessionID[:8]
	}

	return fmt.Sprintf(
		m.wizard.GetString(lang, "status_summary"),
		workspaceID,
		workspacePath,
		mode,
		agentID,
		remoteStatus,
		shortSessionID,
		title,
	) + handoffLine, nil
}
