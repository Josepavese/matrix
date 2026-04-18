package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jose/matrix-v2/internal/logic/sessioncleanup"
	"github.com/jose/matrix-v2/internal/middleware"
)

type newSessionRequest struct {
	ChannelID     string
	Lang          string
	AgentID       string
	WorkspaceID   string
	WorkspacePath string
	Ephemeral     bool
	CleanupPolicy string
}

func (m *Manager) handleSessionNewTyped(req newSessionRequest) (middleware.SessionActionResult, error) {
	resolvedAgentID := m.defaultAgent
	if strings.TrimSpace(req.AgentID) != "" {
		resolvedAgentID = strings.TrimSpace(req.AgentID)
	}
	sessionID, err := m.forceNewSessionWithWorkspacePolicy(newSessionPolicyRequest{
		ChannelID:     req.ChannelID,
		TargetAgent:   resolvedAgentID,
		WorkspaceID:   req.WorkspaceID,
		WorkspacePath: req.WorkspacePath,
		Ephemeral:     req.Ephemeral,
		CleanupPolicy: req.CleanupPolicy,
	})
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	meta, _, _ := m.loadSessionMeta(sessionID)
	return middleware.SessionActionResult{
		Action:          "new",
		Message:         fmt.Sprintf(m.wizard.GetString(req.Lang, "session_new_started"), resolvedAgentID, sessionID),
		ActiveSessionID: sessionID,
		Session:         m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) forceNewSessionWithWorkspace(channelID, targetAgent, workspaceID, workspacePath string) (string, error) {
	return m.forceNewSessionWithWorkspacePolicy(newSessionPolicyRequest{
		ChannelID:     channelID,
		TargetAgent:   targetAgent,
		WorkspaceID:   workspaceID,
		WorkspacePath: workspacePath,
	})
}

type newSessionPolicyRequest struct {
	ChannelID     string
	TargetAgent   string
	WorkspaceID   string
	WorkspacePath string
	Ephemeral     bool
	CleanupPolicy string
}

func (m *Manager) forceNewSessionWithWorkspacePolicy(req newSessionPolicyRequest) (string, error) {
	sessionID := uuid.New().String()
	cleanupPolicy := ""
	if req.Ephemeral || strings.TrimSpace(req.CleanupPolicy) != "" {
		cleanupPolicy = sessioncleanup.NormalizePolicy(req.CleanupPolicy)
	}
	meta := SessionMeta{
		ID:            sessionID,
		CreatedAt:     time.Now().UTC(),
		AgentID:       req.TargetAgent,
		Status:        "active",
		MirrorStatus:  "pending",
		Ephemeral:     req.Ephemeral,
		CleanupPolicy: cleanupPolicy,
	}
	if err := m.bindSessionWorkspace(&meta, req.WorkspaceID, req.WorkspacePath); err != nil {
		return "", fmt.Errorf("failed to bind workspace: %w", err)
	}
	if err := m.saveSessionMeta(meta); err != nil {
		return "", fmt.Errorf("failed to store session meta: %w", err)
	}
	if err := m.updateChannelState(req.ChannelID, sessionID); err != nil {
		return "", fmt.Errorf("failed to store channel mapping: %w", err)
	}
	if err := m.updateChannelWorkspaceState(req.ChannelID, meta.WorkspaceID); err != nil {
		return "", fmt.Errorf("failed to store channel workspace mapping: %w", err)
	}
	if err := m.indexSessionWorkspace(meta); err != nil {
		return "", fmt.Errorf("failed to index session workspace: %w", err)
	}
	m.recordWorkspaceEvent(meta, "session.created", req.ChannelID, "Created workspace session", "session-create", nil)
	return sessionID, nil
}
