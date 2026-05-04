package session

import (
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/workspace"
)

func workspaceBindingEmpty(workspaceID, workspacePath string) bool {
	return workspaceID == "" && strings.TrimSpace(workspacePath) == ""
}

func applyWorkspaceBinding(meta *SessionMeta, workspaceID, workspacePath string) {
	if meta.WorkspaceID != workspaceID || meta.WorkspacePath != workspacePath {
		meta.WorkspaceBoundAt = time.Now().UTC()
	}
	meta.WorkspaceID = workspaceID
	meta.WorkspacePath = workspacePath
	if meta.WorkspaceRole == "" && workspaceID != "" {
		meta.WorkspaceRole = "primary"
	}
}

func (m *Manager) applyWorkspaceModeDefaults(meta *SessionMeta, workspaceID string) {
	if meta.Mode == "" && workspaceID != "" {
		meta.Mode = m.defaultModeForWorkspaceID(workspaceID)
	}
	if meta.Mode == "" {
		meta.Mode = modeImplementation
	}
}

func (m *Manager) defaultModeForWorkspaceID(workspaceID string) string {
	ws, found, err := workspace.LoadMeta(m.storage, workspaceID)
	if err != nil || !found {
		return ""
	}
	return defaultModeForWorkspace(ws)
}
