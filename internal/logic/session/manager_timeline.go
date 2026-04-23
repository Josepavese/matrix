package session

import (
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/workspace"
)

func (m *Manager) recordWorkspaceEvent(meta SessionMeta, eventType, channelID, message, reason string, extra map[string]interface{}) {
	if meta.WorkspaceID == "" {
		return
	}
	metadata := make(map[string]interface{}, len(extra)+1)
	for k, v := range extra {
		metadata[k] = v
	}
	if meta.RemoteStatus != "" && metadata["remote_status"] == nil {
		metadata["remote_status"] = meta.RemoteStatus
	}
	event := workspace.Event{
		WorkspaceID:      meta.WorkspaceID,
		Type:             eventType,
		ChannelID:        channelID,
		LogicalSessionID: meta.ID,
		RemoteSessionID:  meta.AgentSessionID,
		AgentID:          meta.AgentID,
		Mode:             normalizeMode(meta.Mode),
		Message:          message,
		Reason:           reason,
		Metadata:         metadata,
		CreatedAt:        time.Now().UTC(),
	}
	_, _ = workspace.RecordEvent(m.storage, event)
}

func handoffMetadata(meta SessionMeta) map[string]interface{} {
	if meta.LastHandoff == nil {
		return nil
	}
	metadata := map[string]interface{}{}
	if meta.LastHandoff.FromAgentID != "" {
		metadata["from_agent_id"] = meta.LastHandoff.FromAgentID
	}
	if meta.LastHandoff.ToAgentID != "" {
		metadata["to_agent_id"] = meta.LastHandoff.ToAgentID
	}
	if meta.LastHandoff.Summary != "" {
		metadata["summary"] = meta.LastHandoff.Summary
	}
	return metadata
}

func (m *Manager) attachChannelWithEvent(channelID, sessionID, eventType, message, reason string, extra map[string]interface{}) error {
	if err := m.AttachChannel(channelID, sessionID); err != nil {
		return err
	}
	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil || !found {
		return err
	}
	m.recordWorkspaceEvent(meta, eventType, channelID, message, reason, extra)
	return nil
}

func (m *Manager) recordWorkspaceTurn(meta SessionMeta, role, content string) {
	if meta.WorkspaceID == "" || strings.TrimSpace(content) == "" {
		return
	}
	_, _ = workspace.RecordTurn(m.storage, workspace.Turn{
		WorkspaceID:      meta.WorkspaceID,
		LogicalSessionID: meta.ID,
		RemoteSessionID:  meta.AgentSessionID,
		AgentID:          meta.AgentID,
		Role:             role,
		Content:          content,
		CreatedAt:        time.Now().UTC(),
	})
}
