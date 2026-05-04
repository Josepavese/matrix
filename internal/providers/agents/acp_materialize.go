package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (c *acpConversationClient) MaterializeRemoteSession(ctx context.Context, req middleware.SessionMaterializeRequest) (middleware.RemoteSessionInfo, middleware.ConversationMetadata, error) {
	resp, err := c.createACPRemoteSession(ctx, req)
	if err != nil {
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
	resp, err := c.client.NewSession(ctx, acpNewSessionRequest{
		ClientTitle: strings.TrimSpace(req.LogicalSessionID),
		Cwd:         cwd,
		McpServers:  []acpMcpServerConfig{},
		Tools:       toZedACPTools(req.Tools),
	})
	if err != nil {
		return nil, fmt.Errorf("ACP new session failed: %w", err)
	}
	c.markLoadedSession(resp.SessionID)
	return resp, nil
}
