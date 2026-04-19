package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/jose/matrix-v2/internal/middleware"
)

func (m *Manager) AttachRunContext(ctx context.Context, req middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
	base := middleware.RunContextAttachmentResult{Action: "attach_context", DeliveryID: req.DeliveryID}
	logicalID := strings.TrimSpace(req.LogicalSessionID)
	if logicalID == "" {
		return unsupportedAttach(base, "run session is not ready"), nil
	}
	meta, found, err := m.loadSessionMeta(logicalID)
	if err != nil {
		return base, err
	}
	if !found {
		return unsupportedAttach(base, "logical session not found"), nil
	}
	remoteID := firstNonEmpty(meta.AgentSessionID, strings.TrimSpace(req.RemoteSessionID))
	if remoteID == "" {
		return unsupportedAttach(base, "remote session is not ready"), nil
	}
	if req.RemoteSessionID != "" && meta.AgentSessionID != "" && req.RemoteSessionID != meta.AgentSessionID {
		return unsupportedAttach(base, "run remote session does not match active session"), nil
	}
	agentID := firstNonEmpty(meta.AgentID, req.AgentID)
	output, newRemoteID, _, metadata, routeErr := m.router.Route(ctx, middleware.RouteRequest{
		AgentID:          agentID,
		LogicalSessionID: logicalID,
		AgentSessionID:   remoteID,
		WorkspacePath:    firstNonEmpty(meta.WorkspacePath, req.WorkspacePath),
		Message:          "Matrix live context update for run " + strings.TrimSpace(req.RunID) + ". Reason: " + firstNonEmpty(req.Reason, "live_context") + ". Apply attached sidecar context conservatively.",
		SidecarCapsules:  req.SidecarCapsules,
		ThoughtNotifier:  req.Notifier,
	})
	if routeErr != nil {
		return base, fmt.Errorf("live context delivery failed: %w", routeErr)
	}
	meta.AgentID = agentID
	m.persistAgentSession(meta, newRemoteID, metadata, nil)
	base.Status = "delivered"
	base.Message = firstNonEmpty(output, "Live context delivered to active session.")
	return base, nil
}

func unsupportedAttach(result middleware.RunContextAttachmentResult, message string) middleware.RunContextAttachmentResult {
	result.Status = "unsupported"
	result.Message = message
	result.Unsupported = true
	return result
}
