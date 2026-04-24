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

	status := m.statusView(state)
	return fmt.Sprintf(
		m.wizard.GetString(lang, "status_summary"),
		status.WorkspaceID,
		status.WorkspacePath,
		status.Mode,
		status.AgentID,
		status.RemoteStatus,
		status.ShortSessionID(),
		status.Title,
	) + status.HandoffLine, nil
}

type channelStatusView struct {
	WorkspaceID   string
	WorkspacePath string
	AgentID       string
	Mode          string
	RemoteStatus  string
	Title         string
	SessionID     string
	HandoffLine   string
}

func (m *Manager) statusView(state ChannelState) channelStatusView {
	status := channelStatusView{}
	if strings.TrimSpace(state.ActiveSessionID) != "" {
		if meta, found, _ := m.loadSessionMeta(state.ActiveSessionID); found {
			status = statusViewFromMeta(meta)
		}
	}
	status.applyFallbacks(state.PreferredWorkspaceID, m.defaultAgent)
	return status
}

func statusViewFromMeta(meta SessionMeta) channelStatusView {
	status := channelStatusView{
		SessionID:     meta.ID,
		WorkspaceID:   meta.WorkspaceID,
		WorkspacePath: meta.WorkspacePath,
		AgentID:       meta.AgentID,
		Mode:          normalizeMode(meta.Mode),
		RemoteStatus:  meta.RemoteStatus,
		Title:         meta.RemoteTitle,
	}
	if meta.LastHandoff != nil && meta.LastHandoff.FromAgentID != "" {
		status.HandoffLine = fmt.Sprintf("\nHandoff: %s -> %s", meta.LastHandoff.FromAgentID, valueOrDash(meta.LastHandoff.ToAgentID))
	}
	return status
}

func (s *channelStatusView) applyFallbacks(preferredWorkspaceID, defaultAgent string) {
	if s.WorkspaceID == "" {
		s.WorkspaceID = preferredWorkspaceID
	}
	if s.WorkspaceID == "" {
		s.WorkspaceID = "-"
	}
	if s.WorkspacePath == "" {
		s.WorkspacePath = "-"
	}
	if s.AgentID == "" {
		s.AgentID = defaultAgent
	}
	if s.Mode == "" {
		s.Mode = modeImplementation
	}
	if s.RemoteStatus == "" {
		s.RemoteStatus = "active"
	}
	if s.SessionID == "" {
		s.SessionID = "-"
	}
	if s.Title == "" {
		s.Title = "-"
	}
}

func (s channelStatusView) ShortSessionID() string {
	if s.SessionID != "-" && len(s.SessionID) > 8 {
		return s.SessionID[:8]
	}
	return s.SessionID
}
