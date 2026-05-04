package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (c *acpConversationClient) ForkRemoteSession(ctx context.Context, req middleware.SessionForkRequest) (middleware.RemoteSessionInfo, error) {
	if !c.sessionCapabilities.Fork {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("ACP agent does not advertise session/fork")
	}
	parent := strings.TrimSpace(req.RemoteSessionID)
	if parent == "" {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("ACP session id is required")
	}
	cwd := strings.TrimSpace(req.WorkspacePath)
	if cwd == "" {
		cwd = c.cwd
	}
	resp, err := c.client.ForkSession(ctx, acpForkSessionRequest{
		SessionID:  parent,
		Cwd:        cwd,
		McpServers: []acpMcpServerConfig{},
	})
	if err != nil {
		return middleware.RemoteSessionInfo{}, err
	}
	c.markLoadedSession(resp.SessionID)
	return middleware.RemoteSessionInfo{
		RemoteSessionID: resp.SessionID,
		DisplayID:       resp.SessionID,
		ProtocolKind:    middleware.ProtocolKindACP,
		CanResume:       c.sessionCapabilities.Load || c.sessionCapabilities.Resume,
		CanDelete:       c.sessionCapabilities.Delete,
	}, nil
}
