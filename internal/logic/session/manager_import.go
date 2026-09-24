package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/google/uuid"
)

type remoteSessionAttacher interface {
	AttachAgentSessionForWorkspace(context.Context, string, string, string) (middleware.RemoteSessionInfo, error)
}

// handleSessionImportTyped attaches a known external session. It persists a
// mirror only after the provider verifies the exact ID in the requested cwd.
func (m *Manager) handleSessionImportTyped(ctx context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	agentID, remoteID, realPath, err := validateImportRequest(req)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	remote, err := m.verifyImportRemote(ctx, agentID, remoteID, realPath)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	return m.persistVerifiedImport(req, remote, realPath)
}

func validateImportRequest(req middleware.SessionActionRequest) (string, string, string, error) {
	agentID := strings.TrimSpace(req.AgentID)
	remoteID := strings.TrimSpace(req.Target)
	if agentID == "" || remoteID == "" {
		return "", "", "", fmt.Errorf("import requires agent_id and target remote session ID")
	}
	workspacePath := strings.TrimSpace(req.WorkspacePath)
	if !filepath.IsAbs(workspacePath) {
		return "", "", "", fmt.Errorf("import requires an absolute workspace_path")
	}
	realPath, err := filepath.EvalSymlinks(workspacePath)
	if err != nil {
		return "", "", "", fmt.Errorf("workspace_path: %w", err)
	}
	stat, err := os.Stat(realPath)
	if err != nil || !stat.IsDir() {
		return "", "", "", fmt.Errorf("workspace_path is not a directory: %s", realPath)
	}
	return agentID, remoteID, realPath, nil
}

func (m *Manager) verifyImportRemote(ctx context.Context, agentID, remoteID, realPath string) (middleware.RemoteSessionInfo, error) {
	attacher, ok := m.router.(remoteSessionAttacher)
	if !ok {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("verified remote session import is unsupported by this router")
	}
	remote, err := attacher.AttachAgentSessionForWorkspace(ctx, agentID, remoteID, realPath)
	if err != nil {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("remote session %s attach failed: %w", remoteID, err)
	}
	if remote.RemoteSessionID != remoteID {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("provider returned remote session %s instead of %s", remote.RemoteSessionID, remoteID)
	}
	if remote.Cwd != "" {
		remotePath, err := filepath.EvalSymlinks(remote.Cwd)
		if err != nil || remotePath != realPath {
			return middleware.RemoteSessionInfo{}, fmt.Errorf("workspace_mismatch: provider reported %s, requested %s", remote.Cwd, realPath)
		}
	}
	return remote, nil
}

func (m *Manager) persistVerifiedImport(req middleware.SessionActionRequest, remote middleware.RemoteSessionInfo, realPath string) (middleware.SessionActionResult, error) {
	existingID, err := m.findRemoteSessionMirror(strings.TrimSpace(req.AgentID), remote.RemoteSessionID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if existingID != "" {
		return m.refreshImportedMirror(req, existingID, realPath, remote)
	}
	meta, err := m.newImportedMirror(req, remote, realPath)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	sessionID, err := m.persistImportedRemoteSession(req.ChannelID, meta)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	return importActionResult(m, sessionID, meta, remote), nil
}

func (m *Manager) refreshImportedMirror(req middleware.SessionActionRequest, existingID, realPath string, remote middleware.RemoteSessionInfo) (middleware.SessionActionResult, error) {
	meta, found, err := m.loadSessionMeta(existingID)
	if err != nil {
		return middleware.SessionActionResult{}, fmt.Errorf("existing session mirror cannot be loaded: %w", err)
	}
	if !found {
		return middleware.SessionActionResult{}, fmt.Errorf("existing session mirror %s is missing", existingID)
	}
	if meta.WorkspacePath != "" && meta.WorkspacePath != realPath {
		return middleware.SessionActionResult{}, fmt.Errorf("workspace_mismatch: existing mirror uses %s", meta.WorkspacePath)
	}
	meta.WorkspacePath = realPath
	meta.StrictRemote = true
	meta.AdditionalDirectories, err = verifiedRemoteDirectories(remote.AdditionalDirectories, req.AdditionalDirectories, realPath)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if err := m.saveSessionMeta(meta); err != nil {
		return middleware.SessionActionResult{}, err
	}
	if err := m.AttachChannel(req.ChannelID, existingID); err != nil {
		return middleware.SessionActionResult{}, err
	}
	return importActionResult(m, existingID, meta, remote), nil
}

func (m *Manager) newImportedMirror(req middleware.SessionActionRequest, remote middleware.RemoteSessionInfo, realPath string) (SessionMeta, error) {
	now := time.Now().UTC()
	directories, err := verifiedRemoteDirectories(remote.AdditionalDirectories, req.AdditionalDirectories, realPath)
	if err != nil {
		return SessionMeta{}, err
	}
	meta := SessionMeta{ID: uuid.NewString(), AgentSessionID: remote.RemoteSessionID, StrictRemote: true,
		CreatedAt: now, AgentID: strings.TrimSpace(req.AgentID), Status: "active",
		ProtocolKind: string(remote.ProtocolKind), MirrorStatus: "mirrored",
		RemoteTitle: remote.Title, LastSyncedAt: now, WorkspacePath: realPath,
		AdditionalDirectories: directories}
	if req.WorkspaceID != "" {
		if err := m.bindSessionWorkspace(&meta, req.WorkspaceID, realPath); err != nil {
			return SessionMeta{}, err
		}
		if meta.WorkspacePath != realPath {
			return SessionMeta{}, fmt.Errorf("workspace_mismatch: workspace binding changed the requested path")
		}
	}
	return meta, nil
}

func importActionResult(m *Manager, sessionID string, meta SessionMeta, remote middleware.RemoteSessionInfo) middleware.SessionActionResult {
	return middleware.SessionActionResult{Action: "import", ActiveSessionID: sessionID, Session: m.toSessionEntry(meta, true), RemoteSessions: []middleware.RemoteSessionInfo{remote}}
}
