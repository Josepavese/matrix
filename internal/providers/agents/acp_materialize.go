package agents

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// MaterializeRemoteSession creates a remote session without running a turn.
// Like a turn it applies the authentication retry, because session/new is one of
// the requests ACP version 2 gates behind a login.
func (c *acpConversationClient) MaterializeRemoteSession(ctx context.Context, req middleware.SessionMaterializeRequest) (middleware.RemoteSessionInfo, middleware.ConversationMetadata, error) {
	var info middleware.RemoteSessionInfo
	var metadata middleware.ConversationMetadata
	err := c.withAuthenticationRetry(ctx, func() error {
		var createErr error
		info, metadata, createErr = c.materializeRemoteSessionOnce(ctx, req)
		return createErr
	})
	return info, metadata, err
}

func (c *acpConversationClient) materializeRemoteSessionOnce(ctx context.Context, req middleware.SessionMaterializeRequest) (middleware.RemoteSessionInfo, middleware.ConversationMetadata, error) {
	resp, err := c.createACPRemoteSession(ctx, req)
	if err != nil {
		return middleware.RemoteSessionInfo{}, middleware.ConversationMetadata{}, err
	}
	if err := c.applySessionMode(ctx, fromZedACPSession(resp), resp.SessionID, slog.Default()); err != nil {
		return middleware.RemoteSessionInfo{}, middleware.ConversationMetadata{}, err
	}
	info := middleware.RemoteSessionInfo{
		RemoteSessionID: resp.SessionID,
		DisplayID:       resp.SessionID,
		ProtocolKind:    middleware.ProtocolKindACP,
		CanResume:       c.sessionCapabilities.Load || c.sessionCapabilities.Resume,
		CanDelete:       c.sessionCapabilities.Delete,
	}
	return info, middleware.ConversationMetadata{}, nil
}

func (c *acpConversationClient) createACPRemoteSession(ctx context.Context, req middleware.SessionMaterializeRequest) (*acpNewSessionResponse, error) {
	cwd := strings.TrimSpace(req.WorkspacePath)
	if cwd == "" {
		cwd = c.cwd
	}
	additionalDirectories, err := c.additionalDirectories(req.AdditionalDirectories)
	if err != nil {
		return nil, err
	}
	mcpServers, err := c.materializeMCPServers(req.McpServers)
	if err != nil {
		return nil, err
	}
	resp, err := c.currentACPClient().NewSession(ctx, acpNewSessionRequest{
		ClientTitle:           strings.TrimSpace(req.LogicalSessionID),
		Cwd:                   cwd,
		AdditionalDirectories: additionalDirectories,
		McpServers:            mcpServers,
		Tools:                 toZedACPTools(req.Tools),
	})
	if err != nil {
		return nil, fmt.Errorf("ACP new session failed: %w", err)
	}
	c.markLoadedSession(resp.SessionID)
	return resp, nil
}

func (c *acpConversationClient) materializeMCPServers(reqServers []middleware.McpServerConfig) ([]acpMcpServerConfig, error) {
	var servers []acpMcpServerConfig
	if len(reqServers) > 0 {
		servers = toZedACPMCPServers(reqServers)
	} else {
		servers = cloneACPMCPServers(c.mcpServers)
	}
	if err := c.validateMCPServers(servers); err != nil {
		return nil, err
	}
	return servers, nil
}
