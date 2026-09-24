package agents

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (c *acpConversationClient) ListRemoteSessions(ctx context.Context) ([]middleware.RemoteSessionInfo, error) {
	if !c.sessionCapabilities.List {
		return nil, fmt.Errorf("ACP agent does not advertise session/list")
	}
	var out []middleware.RemoteSessionInfo
	cursor := ""
	for page := 0; page < 100; page++ {
		resp, err := c.currentACPClient().ListSessionsWithRequest(ctx, acpListSessionsRequest{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, session := range resp.Sessions {
			out = append(out, c.remoteSessionInfo(session))
		}
		if strings.TrimSpace(resp.NextCursor) == "" {
			return out, nil
		}
		cursor = resp.NextCursor
	}
	return nil, fmt.Errorf("ACP session/list pagination exceeded safety limit")
}

func (c *acpConversationClient) remoteSessionInfo(session acpSessionInfo) middleware.RemoteSessionInfo {
	return middleware.RemoteSessionInfo{
		RemoteSessionID:       session.SessionID,
		DisplayID:             session.SessionID,
		Title:                 session.Title,
		UpdatedAt:             session.UpdatedAt,
		Cwd:                   session.Cwd,
		AdditionalDirectories: append([]string(nil), session.AdditionalDirectories...),
		ProtocolKind:          middleware.ProtocolKindACP,
		CanResume:             c.sessionCapabilities.Load || c.sessionCapabilities.Resume,
		CanDelete:             c.sessionCapabilities.Delete,
	}
}

// AttachExistingRemoteSession never creates a session and never sends a prompt.
// A successful resume/load RPC is the provider's proof for this exact ID.
func (c *acpConversationClient) AttachExistingRemoteSession(ctx context.Context, remoteSessionID, workspacePath string) (middleware.RemoteSessionInfo, error) {
	remoteSessionID = strings.TrimSpace(remoteSessionID)
	if remoteSessionID == "" {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("remote session ID is required")
	}
	if !c.sessionCapabilities.Resume && !c.sessionCapabilities.Load {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("ACP agent does not support session/resume or session/load")
	}
	if workspacePath == "" {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("workspace path is required for verified attach")
	}
	info, err := c.externalSessionInfo(ctx, remoteSessionID, workspacePath)
	if err != nil {
		return middleware.RemoteSessionInfo{}, err
	}
	method, err := c.verifyExistingSession(ctx, remoteSessionID, workspacePath)
	if err != nil {
		if info.ListWarning != "" {
			return middleware.RemoteSessionInfo{}, fmt.Errorf("%s; %w", info.ListWarning, err)
		}
		return middleware.RemoteSessionInfo{}, err
	}
	c.markLoadedSession(remoteSessionID)
	info.VerificationMethod = method
	return info, nil
}

func (c *acpConversationClient) externalSessionInfo(ctx context.Context, remoteSessionID, workspacePath string) (middleware.RemoteSessionInfo, error) {
	info := middleware.RemoteSessionInfo{
		RemoteSessionID: remoteSessionID, DisplayID: remoteSessionID,
		ProtocolKind: middleware.ProtocolKindACP, CanResume: true,
		CanDelete: c.sessionCapabilities.Delete, Cwd: workspacePath,
		VerificationLimit: "provider accepted the exact remote ID; Matrix cannot inspect or attest the conversation history",
	}
	if !c.sessionCapabilities.List {
		info.ListWarning = "session/list unsupported"
		return info, nil
	}
	listed, err := c.ListRemoteSessions(ctx)
	if err != nil {
		info.ListWarning = fmt.Sprintf("session/list failed: %v", err)
		return info, nil
	}
	for _, candidate := range listed {
		if candidate.RemoteSessionID != remoteSessionID {
			continue
		}
		if err := verifyListedSessionCwd(candidate.Cwd, workspacePath); err != nil {
			return middleware.RemoteSessionInfo{}, fmt.Errorf("workspace_mismatch: session %s reports %s, requested %s", remoteSessionID, candidate.Cwd, workspacePath)
		}
		candidate.VerificationLimit = info.VerificationLimit
		if candidate.Cwd == "" {
			candidate.Cwd = workspacePath
		}
		return candidate, nil
	}
	return info, nil
}

func verifyListedSessionCwd(reported, requested string) error {
	if reported == "" {
		return nil
	}
	if !filepath.IsAbs(reported) {
		return fmt.Errorf("provider reported relative workspace")
	}
	physical, err := filepath.EvalSymlinks(reported)
	if err != nil {
		return err
	}
	if filepath.Clean(physical) != filepath.Clean(requested) {
		return fmt.Errorf("different workspace")
	}
	return nil
}

func (c *acpConversationClient) verifyExistingSession(ctx context.Context, remoteSessionID, workspacePath string) (string, error) {
	var resumeErr error
	if c.sessionCapabilities.Resume {
		resp, err := c.currentACPClient().ResumeSession(ctx, acpResumeSessionRequest{SessionID: remoteSessionID, Cwd: workspacePath, McpServers: cloneACPMCPServers(c.mcpServers)})
		if err == nil {
			if err := c.applySessionMode(ctx, fromZedACPResumeSession(resp), remoteSessionID, slog.Default()); err != nil {
				return "", err
			}
			return "session/resume", nil
		}
		resumeErr = err
	}
	if !c.sessionCapabilities.Load {
		return "", fmt.Errorf("provider_failure: resume: %w", resumeErr)
	}
	resp, err := c.currentACPClient().LoadSession(ctx, acpLoadSessionRequest{SessionID: remoteSessionID, Cwd: workspacePath, McpServers: cloneACPMCPServers(c.mcpServers)}, nil)
	if err != nil {
		if resumeErr != nil {
			return "", fmt.Errorf("provider_failure: resume: %v; load: %w", resumeErr, err)
		}
		return "", fmt.Errorf("provider_failure: load: %w", err)
	}
	if err := c.applySessionMode(ctx, fromZedACPLoadSession(resp), remoteSessionID, slog.Default()); err != nil {
		return "", err
	}
	return "session/load", nil
}

func (c *acpConversationClient) GetRemoteSession(ctx context.Context, remoteSessionID string) (middleware.RemoteSessionInfo, error) {
	if c.sessionCapabilities.List {
		sessions, err := c.ListRemoteSessions(ctx)
		if err != nil {
			return middleware.RemoteSessionInfo{}, err
		}
		for _, session := range sessions {
			if session.RemoteSessionID == remoteSessionID || session.DisplayID == remoteSessionID {
				return session, nil
			}
		}
	}
	if c.sessionCapabilities.Resume {
		if _, err := c.currentACPClient().ResumeSession(ctx, acpResumeSessionRequest{
			SessionID:  remoteSessionID,
			Cwd:        c.cwd,
			McpServers: cloneACPMCPServers(c.mcpServers),
		}); err == nil {
			c.markLoadedSession(remoteSessionID)
			return middleware.RemoteSessionInfo{
				RemoteSessionID: remoteSessionID,
				DisplayID:       remoteSessionID,
				ProtocolKind:    middleware.ProtocolKindACP,
				CanResume:       true,
				CanDelete:       c.sessionCapabilities.Delete,
			}, nil
		}
	}
	if c.sessionCapabilities.Load {
		if _, err := c.currentACPClient().LoadSession(ctx, acpLoadSessionRequest{
			SessionID:  remoteSessionID,
			Cwd:        c.cwd,
			McpServers: cloneACPMCPServers(c.mcpServers),
		}, nil); err == nil {
			c.markLoadedSession(remoteSessionID)
			return middleware.RemoteSessionInfo{
				RemoteSessionID: remoteSessionID,
				DisplayID:       remoteSessionID,
				ProtocolKind:    middleware.ProtocolKindACP,
				CanResume:       c.sessionCapabilities.Load || c.sessionCapabilities.Resume,
				CanDelete:       c.sessionCapabilities.Delete,
			}, nil
		}
	}
	return middleware.RemoteSessionInfo{}, fmt.Errorf("ACP session %s not found", remoteSessionID)
}
