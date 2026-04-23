package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
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
	runRemoteID := strings.TrimSpace(req.RemoteSessionID)
	metaRemoteID := strings.TrimSpace(meta.AgentSessionID)
	// For live attach, the run trace is the active execution contract. The
	// vault mirror can lag until the active turn finishes and persists metadata.
	remoteID := firstNonEmpty(runRemoteID, metaRemoteID)
	if remoteID == "" {
		return unsupportedAttach(base, "remote session is not ready"), nil
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
		StrictSession:    runRemoteID != "",
	})
	if routeErr != nil {
		return base, fmt.Errorf("live context delivery failed: %w", routeErr)
	}
	if runRemoteID != "" && newRemoteID != "" && newRemoteID != runRemoteID {
		return unsupportedAttach(base, "live context delivery changed remote session"), nil
	}
	meta.AgentID = agentID
	m.persistAgentSession(meta, firstNonEmpty(newRemoteID, remoteID), metadata, nil)
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
