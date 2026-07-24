package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (m *Manager) CancelRun(ctx context.Context, req middleware.RunCancellationRequest) error {
	if strings.TrimSpace(req.RemoteSessionID) == "" {
		return nil
	}
	if controller, ok := m.router.(middleware.AgentWorkspaceSessionController); ok && strings.TrimSpace(req.WorkspacePath) != "" {
		return controller.CancelAgentSessionForWorkspace(ctx, req.AgentID, req.RemoteSessionID, req.WorkspacePath)
	}
	controller, ok := m.router.(middleware.AgentSessionController)
	if !ok {
		return fmt.Errorf("agent router does not expose remote session control")
	}
	return controller.CancelAgentSession(ctx, req.AgentID, req.RemoteSessionID)
}
