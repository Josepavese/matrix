package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
)

func (m *Manager) RouteConversation(ctx context.Context, req middleware.ConversationRequest) (string, error) {
	if !m.wizard.IsConfigured() {
		if req.NonInteractive {
			return "", errors.Join(middleware.ErrSetupRequired, fmt.Errorf("system.configured is false or missing"))
		}
		return m.wizard.Process(req.ChannelID, req.Input)
	}
	if !req.NonInteractive {
		if handled, response, err := m.tryHandleCommand(ctx, req.ChannelID, req.Input); handled {
			return response, err
		}
	}
	return m.routeAgentTurnWithWorkspace(ctx, req)
}

func (m *Manager) routeAgentTurnWithWorkspace(ctx context.Context, req middleware.ConversationRequest) (string, error) {
	if output, handled, err := m.routeExplicitLogicalSession(ctx, req); handled || err != nil {
		return output, err
	}
	routeReq, err := m.conversationWorkspaceRoute(req)
	if err != nil {
		return "", err
	}
	sessionID, decision, err := m.getOrCreateSessionForWorkspace(routeReq.ChannelID, routeReq.TargetAgent, routeReq.WorkspaceID, routeReq.WorkspacePath)
	if err != nil {
		return "", fmt.Errorf("failed to route session: %w", err)
	}
	m.recordWorkspaceRouteDecision(sessionID, routeReq.ChannelID, decision)
	return m.routeResolvedSession(ctx, req, sessionID, routeReq.TargetAgent)
}

type workspaceRouteRequest struct {
	ChannelID     string
	TargetAgent   string
	WorkspaceID   string
	WorkspacePath string
}

func (m *Manager) getOrCreateSessionForWorkspace(channelID, targetAgent, workspaceID, workspacePath string) (string, *routeDecision, error) {
	req := workspaceRouteRequest{
		ChannelID:     channelID,
		TargetAgent:   strings.TrimSpace(targetAgent),
		WorkspaceID:   strings.TrimSpace(workspaceID),
		WorkspacePath: strings.TrimSpace(workspacePath),
	}
	state, stateErr := m.getChannelState(req.ChannelID)
	if sessionID, decision, reused, err := m.reuseActiveWorkspaceSession(req, state, stateErr); err != nil || reused {
		return sessionID, decision, err
	}
	if sessionID, decision, resumed, err := m.resumeIndexedWorkspaceSession(req); err != nil || resumed {
		return sessionID, decision, err
	}
	if stateErr == nil && strings.TrimSpace(state.PreferredWorkspaceID) != "" && req.WorkspaceID == "" {
		req.WorkspaceID = state.PreferredWorkspaceID
		return m.getOrCreateSessionForWorkspace(req.ChannelID, req.TargetAgent, req.WorkspaceID, req.WorkspacePath)
	}
	return m.createWorkspaceSession(req)
}

func (m *Manager) reuseActiveWorkspaceSession(req workspaceRouteRequest, state ChannelState, stateErr error) (string, *routeDecision, bool, error) {
	if !canReuseActiveState(state, stateErr) {
		return "", nil, false, nil
	}
	meta, found := m.loadReusableSessionMeta(state.ActiveSessionID)
	if !found || !workspaceRouteMatches(meta, req) {
		return "", nil, false, nil
	}
	if err := m.updateChannelWorkspaceState(req.ChannelID, meta.WorkspaceID); err != nil {
		return "", nil, true, err
	}
	return state.ActiveSessionID, reuseActiveDecision(req, meta, state.ActiveSessionID), true, nil
}

func canReuseActiveState(state ChannelState, stateErr error) bool {
	return stateErr == nil && strings.TrimSpace(state.ActiveSessionID) != ""
}

func (m *Manager) loadReusableSessionMeta(sessionID string) (SessionMeta, bool) {
	meta, found, err := m.loadSessionMeta(sessionID)
	return meta, err == nil && found
}

func workspaceRouteMatches(meta SessionMeta, req workspaceRouteRequest) bool {
	return sessionMatchesWorkspaceHints(meta, req.WorkspaceID, req.WorkspacePath) &&
		(req.TargetAgent == "" || meta.AgentID == req.TargetAgent)
}

func reuseActiveDecision(req workspaceRouteRequest, meta SessionMeta, sessionID string) *routeDecision {
	return &routeDecision{
		Kind:              "reuse-active-session",
		Source:            "channel-active",
		Explanation:       "Reused the channel's active session because it already matched the requested workspace and agent.",
		RequestedAgentID:  req.TargetAgent,
		SelectedAgentID:   meta.AgentID,
		SelectedSessionID: sessionID,
		SelectedMode:      normalizeMode(meta.Mode),
	}
}

func (m *Manager) resumeIndexedWorkspaceSession(req workspaceRouteRequest) (string, *routeDecision, bool, error) {
	if strings.TrimSpace(req.WorkspaceID) == "" {
		return "", nil, false, nil
	}
	sessionIDs, err := workspace.LoadSessionIndex(m.storage, req.WorkspaceID)
	if err != nil {
		return "", nil, true, err
	}
	for _, sessionID := range sessionIDs {
		candidate, ok := m.workspaceSessionCandidate(req, sessionID)
		if !ok {
			continue
		}
		if req.TargetAgent != "" && candidate.Meta.AgentID != req.TargetAgent {
			continue
		}
		return m.resumeWorkspaceSession(req, candidate)
	}
	return "", nil, false, nil
}

type workspaceSessionCandidate struct {
	SessionID string
	Meta      SessionMeta
}

func (m *Manager) workspaceSessionCandidate(req workspaceRouteRequest, sessionID string) (workspaceSessionCandidate, bool) {
	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil || !found || strings.TrimSpace(meta.WorkspaceID) != req.WorkspaceID {
		return workspaceSessionCandidate{}, false
	}
	return workspaceSessionCandidate{SessionID: sessionID, Meta: meta}, true
}

func (m *Manager) resumeWorkspaceSession(req workspaceRouteRequest, candidate workspaceSessionCandidate) (string, *routeDecision, bool, error) {
	if err := m.attachChannelWithEvent(req.ChannelID, candidate.SessionID, "session.resumed", "Resumed workspace session", "workspace-resume", nil); err != nil {
		return "", nil, true, err
	}
	if err := m.updateChannelWorkspaceState(req.ChannelID, req.WorkspaceID); err != nil {
		return "", nil, true, err
	}
	return candidate.SessionID, resumeWorkspaceDecision(req, candidate), true, nil
}

func resumeWorkspaceDecision(req workspaceRouteRequest, candidate workspaceSessionCandidate) *routeDecision {
	return &routeDecision{
		Kind:              "resume-workspace-session",
		Source:            "workspace-session-index",
		Explanation:       "Resumed an existing session from the workspace because it matched the requested agent.",
		RequestedAgentID:  req.TargetAgent,
		SelectedAgentID:   candidate.Meta.AgentID,
		SelectedSessionID: candidate.SessionID,
		SelectedMode:      normalizeMode(candidate.Meta.Mode),
	}
}

func (m *Manager) createWorkspaceSession(req workspaceRouteRequest) (string, *routeDecision, error) {
	resolvedAgent := m.resolveWorkspaceRouteAgent(req)
	sessionID, err := m.forceNewSessionWithWorkspace(req.ChannelID, resolvedAgent, req.WorkspaceID, req.WorkspacePath)
	if err != nil {
		return "", nil, err
	}
	return sessionID, createWorkspaceDecision(req, resolvedAgent, sessionID), nil
}

func (m *Manager) resolveWorkspaceRouteAgent(req workspaceRouteRequest) string {
	if req.TargetAgent != "" {
		return req.TargetAgent
	}
	if req.WorkspaceID != "" {
		if ws, found, err := workspace.LoadMeta(m.storage, req.WorkspaceID); err == nil && found && strings.TrimSpace(ws.DefaultAgentID) != "" {
			return ws.DefaultAgentID
		}
	}
	return m.defaultAgent
}

func createWorkspaceDecision(req workspaceRouteRequest, resolvedAgent, sessionID string) *routeDecision {
	source := "requested-agent"
	explanation := "Created a new session for the explicitly requested agent."
	if req.TargetAgent == "" && req.WorkspaceID != "" {
		source = "workspace-default-agent"
		explanation = "Created a new session using the workspace default agent because no explicit agent was requested."
	}
	if req.TargetAgent == "" && req.WorkspaceID == "" {
		source = "global-default-agent"
		explanation = "Created a new session using the global default agent because no explicit agent or workspace default was available."
	}
	return &routeDecision{
		Kind:              "create-session",
		Source:            source,
		Explanation:       explanation,
		RequestedAgentID:  req.TargetAgent,
		SelectedAgentID:   resolvedAgent,
		SelectedSessionID: sessionID,
		SelectedMode:      modeImplementation,
	}
}

func (m *Manager) bindSessionWorkspace(meta *SessionMeta, workspaceID, workspacePath string) error {
	resolvedID, resolvedPath, err := m.resolveWorkspaceHint(workspaceID, workspacePath)
	if err != nil {
		return err
	}
	if workspaceBindingEmpty(resolvedID, resolvedPath) {
		return nil
	}
	applyWorkspaceBinding(meta, resolvedID, resolvedPath)
	m.applyWorkspaceModeDefaults(meta, resolvedID)
	return nil
}

func sessionMatchesWorkspaceHints(meta SessionMeta, workspaceID, workspacePath string) bool {
	if strings.TrimSpace(workspaceID) != "" && meta.WorkspaceID != workspaceID {
		return false
	}
	if strings.TrimSpace(workspacePath) != "" && filepath.Clean(meta.WorkspacePath) != filepath.Clean(workspacePath) {
		return false
	}
	return true
}

func (m *Manager) resolveWorkspaceHint(workspaceID, workspacePath string) (string, string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	workspacePath = strings.TrimSpace(workspacePath)
	if workspaceID != "" {
		meta, found, err := workspace.LoadMeta(m.storage, workspaceID)
		if err != nil {
			return "", "", err
		}
		if !found {
			return "", "", fmt.Errorf("workspace %s not found", workspaceID)
		}
		if workspacePath == "" {
			workspacePath = meta.RootPath
		}
		return meta.ID, workspacePath, nil
	}
	if workspacePath != "" {
		clean := filepath.Clean(workspacePath)
		if meta, found, err := workspace.ResolveByPath(m.storage, clean); err != nil {
			return "", "", err
		} else if found {
			return meta.ID, clean, nil
		}
		return "", clean, nil
	}
	return "", "", nil
}

func (m *Manager) indexSessionWorkspace(meta SessionMeta) error {
	if strings.TrimSpace(meta.WorkspaceID) == "" {
		return nil
	}
	return workspace.UpdateSessionIndex(m.storage, meta.WorkspaceID, meta.ID)
}
