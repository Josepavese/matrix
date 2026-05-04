package session

import (
	"context"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
)

func (m *Manager) routeExplicitLogicalSession(ctx context.Context, req middleware.ConversationRequest) (string, bool, error) {
	sessionID := strings.TrimSpace(req.LogicalSessionID)
	if sessionID == "" {
		return "", false, nil
	}
	meta, err := m.loadRequiredSessionMeta(sessionID, "session")
	if err != nil {
		return "", true, err
	}
	output, err := m.routeResolvedSession(ctx, req, sessionID, meta.AgentID)
	return output, true, err
}

func (m *Manager) conversationWorkspaceRoute(req middleware.ConversationRequest) (workspaceRouteRequest, error) {
	workspaceID, workspacePath, err := m.resolveWorkspaceHint(req.WorkspaceID, req.WorkspacePath)
	if err != nil {
		return workspaceRouteRequest{}, err
	}
	agentID := firstNonEmpty(req.AgentID, m.workspaceDefaultAgent(workspaceID))
	return workspaceRouteRequest{ChannelID: req.ChannelID, TargetAgent: agentID, WorkspaceID: workspaceID, WorkspacePath: workspacePath}, nil
}

func (m *Manager) workspaceDefaultAgent(workspaceID string) string {
	if strings.TrimSpace(workspaceID) == "" {
		return ""
	}
	ws, found, err := workspace.LoadMeta(m.storage, workspaceID)
	if err != nil || !found {
		return ""
	}
	return strings.TrimSpace(ws.DefaultAgentID)
}

func (m *Manager) recordWorkspaceRouteDecision(sessionID, channelID string, decision *routeDecision) {
	if decision == nil {
		return
	}
	meta, found, err := m.loadSessionMeta(sessionID)
	if err == nil && found {
		m.recordWorkspaceDecision(meta, channelID, decision)
	}
}
