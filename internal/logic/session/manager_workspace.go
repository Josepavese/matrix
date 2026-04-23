package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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
	if sessionID := strings.TrimSpace(req.LogicalSessionID); sessionID != "" {
		meta, found, err := m.loadSessionMeta(sessionID)
		if err != nil {
			return "", err
		}
		if !found {
			return "", fmt.Errorf("session %s not found", sessionID)
		}
		return m.routeResolvedSession(ctx, req, sessionID, meta.AgentID)
	}
	workspaceID, workspacePath, err := m.resolveWorkspaceHint(req.WorkspaceID, req.WorkspacePath)
	if err != nil {
		return "", err
	}
	agentID := strings.TrimSpace(req.AgentID)
	if workspaceID != "" {
		if ws, found, err := workspace.LoadMeta(m.storage, workspaceID); err == nil && found && strings.TrimSpace(ws.DefaultAgentID) != "" && req.AgentID == "" {
			agentID = ws.DefaultAgentID
		}
	}
	sessionID, decision, err := m.getOrCreateSessionForWorkspace(req.ChannelID, agentID, workspaceID, workspacePath)
	if err != nil {
		return "", fmt.Errorf("failed to route session: %w", err)
	}
	if decision != nil {
		if meta, found, loadErr := m.loadSessionMeta(sessionID); loadErr == nil && found {
			m.recordWorkspaceDecision(meta, req.ChannelID, decision)
		}
	}
	return m.routeResolvedSession(ctx, req, sessionID, agentID)
}

func (m *Manager) getOrCreateSessionForWorkspace(channelID, targetAgent, workspaceID, workspacePath string) (string, *routeDecision, error) {
	state, err := m.getChannelState(channelID)
	if err == nil && state.ActiveSessionID != "" {
		meta, found, metaErr := m.loadSessionMeta(state.ActiveSessionID)
		if metaErr == nil && found {
			if sessionMatchesWorkspaceHints(meta, workspaceID, workspacePath) && (strings.TrimSpace(targetAgent) == "" || meta.AgentID == targetAgent) {
				if err := m.updateChannelWorkspaceState(channelID, meta.WorkspaceID); err != nil {
					return "", nil, err
				}
				return state.ActiveSessionID, &routeDecision{
					Kind:              "reuse-active-session",
					Source:            "channel-active",
					Explanation:       "Reused the channel's active session because it already matched the requested workspace and agent.",
					RequestedAgentID:  strings.TrimSpace(targetAgent),
					SelectedAgentID:   meta.AgentID,
					SelectedSessionID: state.ActiveSessionID,
					SelectedMode:      normalizeMode(meta.Mode),
				}, nil
			}
		}
	}

	if strings.TrimSpace(workspaceID) != "" {
		sessionIDs, err := workspace.LoadSessionIndex(m.storage, workspaceID)
		if err != nil {
			return "", nil, err
		}
		var fallbackSessionID string
		for _, sessionID := range sessionIDs {
			meta, found, err := m.loadSessionMeta(sessionID)
			if err != nil || !found {
				continue
			}
			if strings.TrimSpace(meta.WorkspaceID) != workspaceID {
				continue
			}
			if strings.TrimSpace(targetAgent) != "" && meta.AgentID != targetAgent {
				if fallbackSessionID == "" {
					fallbackSessionID = sessionID
				}
				continue
			}
			if err := m.attachChannelWithEvent(channelID, sessionID, "session.resumed", "Resumed workspace session", "workspace-resume", nil); err != nil {
				return "", nil, err
			}
			if err := m.updateChannelWorkspaceState(channelID, workspaceID); err != nil {
				return "", nil, err
			}
			return sessionID, &routeDecision{
				Kind:              "resume-workspace-session",
				Source:            "workspace-session-index",
				Explanation:       "Resumed an existing session from the workspace because it matched the requested agent.",
				RequestedAgentID:  strings.TrimSpace(targetAgent),
				SelectedAgentID:   meta.AgentID,
				SelectedSessionID: sessionID,
				SelectedMode:      normalizeMode(meta.Mode),
			}, nil
		}
		if strings.TrimSpace(targetAgent) == "" && fallbackSessionID != "" {
			if err := m.attachChannelWithEvent(channelID, fallbackSessionID, "session.resumed", "Resumed workspace session", "workspace-resume", nil); err != nil {
				return "", nil, err
			}
			if err := m.updateChannelWorkspaceState(channelID, workspaceID); err != nil {
				return "", nil, err
			}
			meta, found, loadErr := m.loadSessionMeta(fallbackSessionID)
			if loadErr != nil {
				return "", nil, loadErr
			}
			if !found {
				return "", nil, fmt.Errorf("workspace session %s not found", fallbackSessionID)
			}
			return fallbackSessionID, &routeDecision{
				Kind:              "resume-workspace-session",
				Source:            "workspace-session-index-fallback",
				Explanation:       "Resumed the most recent workspace session because no explicit agent was requested.",
				RequestedAgentID:  strings.TrimSpace(targetAgent),
				SelectedAgentID:   meta.AgentID,
				SelectedSessionID: fallbackSessionID,
				SelectedMode:      normalizeMode(meta.Mode),
				FallbackUsed:      true,
			}, nil
		}
	}

	if err == nil && strings.TrimSpace(state.PreferredWorkspaceID) != "" && workspaceID == "" {
		return m.getOrCreateSessionForWorkspace(channelID, targetAgent, state.PreferredWorkspaceID, workspacePath)
	}

	resolvedAgent := strings.TrimSpace(targetAgent)
	if resolvedAgent == "" && strings.TrimSpace(workspaceID) != "" {
		if ws, found, err := workspace.LoadMeta(m.storage, workspaceID); err == nil && found && strings.TrimSpace(ws.DefaultAgentID) != "" {
			resolvedAgent = ws.DefaultAgentID
		}
	}
	if resolvedAgent == "" {
		resolvedAgent = m.defaultAgent
	}
	sessionID, err := m.forceNewSessionWithWorkspace(channelID, resolvedAgent, workspaceID, workspacePath)
	if err != nil {
		return "", nil, err
	}
	source := "requested-agent"
	explanation := "Created a new session for the explicitly requested agent."
	if strings.TrimSpace(targetAgent) == "" && strings.TrimSpace(workspaceID) != "" {
		source = "workspace-default-agent"
		explanation = "Created a new session using the workspace default agent because no explicit agent was requested."
	}
	if strings.TrimSpace(targetAgent) == "" && strings.TrimSpace(workspaceID) == "" {
		source = "global-default-agent"
		explanation = "Created a new session using the global default agent because no explicit agent or workspace default was available."
	}
	return sessionID, &routeDecision{
		Kind:              "create-session",
		Source:            source,
		Explanation:       explanation,
		RequestedAgentID:  strings.TrimSpace(targetAgent),
		SelectedAgentID:   resolvedAgent,
		SelectedSessionID: sessionID,
		SelectedMode:      modeImplementation,
	}, nil
}

func (m *Manager) bindSessionWorkspace(meta *SessionMeta, workspaceID, workspacePath string) error {
	resolvedID, resolvedPath, err := m.resolveWorkspaceHint(workspaceID, workspacePath)
	if err != nil {
		return err
	}
	if resolvedID == "" && strings.TrimSpace(resolvedPath) == "" {
		return nil
	}
	if meta.WorkspaceID != resolvedID || meta.WorkspacePath != resolvedPath {
		meta.WorkspaceBoundAt = time.Now().UTC()
	}
	meta.WorkspaceID = resolvedID
	meta.WorkspacePath = resolvedPath
	if meta.WorkspaceRole == "" && resolvedID != "" {
		meta.WorkspaceRole = "primary"
	}
	if meta.Mode == "" && resolvedID != "" {
		if ws, found, err := workspace.LoadMeta(m.storage, resolvedID); err == nil && found {
			meta.Mode = defaultModeForWorkspace(ws)
		}
	}
	if meta.Mode == "" {
		meta.Mode = modeImplementation
	}
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
