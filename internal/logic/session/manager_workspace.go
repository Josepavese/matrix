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
	plan, err := m.planWorkspaceRoute(newWorkspaceRouteRequest(channelID, targetAgent, workspaceID, workspacePath))
	if err != nil {
		return "", nil, err
	}
	return m.applyWorkspaceRoutePlan(plan)
}

// applyWorkspaceRoutePlan performs the side effects of an already resolved plan.
// Splitting plan from apply is what lets a caller publish the same decision
// before the prompt instead of describing it after the fact.
func (m *Manager) applyWorkspaceRoutePlan(plan workspaceRoutePlan) (string, *routeDecision, error) {
	switch plan.kind {
	case workspaceRouteReuseActive:
		if err := m.updateChannelWorkspaceState(plan.request.ChannelID, plan.meta.WorkspaceID); err != nil {
			return "", nil, err
		}
		return plan.sessionID, reuseActiveDecision(plan.request, plan.meta, plan.sessionID), nil
	case workspaceRouteResumeIndexed:
		if err := m.attachChannelWithEvent(plan.request.ChannelID, plan.sessionID, "session.resumed", "Resumed workspace session", "workspace-resume", nil); err != nil {
			return "", nil, err
		}
		if err := m.updateChannelWorkspaceState(plan.request.ChannelID, plan.request.WorkspaceID); err != nil {
			return "", nil, err
		}
		candidate := workspaceSessionCandidate{SessionID: plan.sessionID, Meta: plan.meta}
		return plan.sessionID, resumeWorkspaceDecision(plan.request, candidate), nil
	default:
		return m.createWorkspaceSession(plan.request)
	}
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

type workspaceSessionCandidate struct {
	SessionID string
	Meta      SessionMeta
}

func (m *Manager) workspaceSessionCandidate(req workspaceRouteRequest, sessionID string) (workspaceSessionCandidate, bool) {
	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil || !found || strings.TrimSpace(meta.WorkspaceID) != req.WorkspaceID {
		return workspaceSessionCandidate{}, false
	}
	if !sessionWorkspaceAffinityMatches(meta, req.WorkspacePath) {
		return workspaceSessionCandidate{}, false
	}
	return workspaceSessionCandidate{SessionID: sessionID, Meta: meta}, true
}

// sessionWorkspaceAffinityMatches refuses a session whose recorded workspace
// path belongs to a different workspace than the one being requested. A session
// that recorded no path is unknown, not a different workspace.
func sessionWorkspaceAffinityMatches(meta SessionMeta, requestedPath string) bool {
	requestedPath = strings.TrimSpace(requestedPath)
	recordedPath := strings.TrimSpace(meta.WorkspacePath)
	if requestedPath == "" || recordedPath == "" {
		return true
	}
	return filepath.Clean(recordedPath) == filepath.Clean(requestedPath)
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

// resolveWorkspaceHint delegates to the workspace identity contract: one
// canonical answer for the pair, and a typed refusal when the two hints denote
// different workspaces instead of a silent ambiguous binding.
func (m *Manager) resolveWorkspaceHint(workspaceID, workspacePath string) (string, string, error) {
	identity, err := workspace.ResolveIdentity(m.storage, workspaceID, workspacePath)
	if err != nil {
		return "", "", err
	}
	return identity.ID, identity.Path, nil
}

func (m *Manager) indexSessionWorkspace(meta SessionMeta) error {
	if strings.TrimSpace(meta.WorkspaceID) == "" {
		return nil
	}
	return workspace.UpdateSessionIndex(m.storage, meta.WorkspaceID, meta.ID)
}
