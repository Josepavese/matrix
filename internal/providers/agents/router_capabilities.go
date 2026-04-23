package agents

import (
	"context"
	"fmt"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (r *Router) AgentCapabilities(ctx context.Context, agentID string) (middleware.ProviderCapabilityReport, error) {
	endpoint, err := r.resolver.GetAgentEndpoint(agentID)
	if err != nil {
		return middleware.ProviderCapabilityReport{}, err
	}
	report := middleware.ProviderCapabilityReport{AgentID: agentID, ProtocolKind: endpoint.Kind}
	client, err := r.getOrCreateSessionControlClient(ctx, agentID)
	if err != nil {
		return middleware.ProviderCapabilityReport{}, err
	}
	controller, ok := client.(middleware.ConversationSessionControl)
	if !ok {
		report.Session = unsupportedSessionCapabilityDetails()
		return report, nil
	}
	caps := controller.SessionCapabilities()
	report.Session = caps.Details
	if len(report.Session) == 0 {
		report.Session = capabilityDetailsFromBooleans(caps)
	}
	return report, nil
}

func (r *Router) ForkAgentSession(ctx context.Context, agentID string, req middleware.SessionForkRequest) (middleware.RemoteSessionInfo, error) {
	client, err := r.getOrCreateSessionControlClientForWorkspace(ctx, agentID, req.WorkspacePath)
	if err != nil {
		return middleware.RemoteSessionInfo{}, err
	}
	forker, ok := client.(middleware.ConversationSessionForker)
	if !ok {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("agent %s does not expose remote session fork", agentID)
	}
	return forker.ForkRemoteSession(ctx, req)
}

func unsupportedSessionCapabilityDetails() map[string]middleware.CapabilityDescriptor {
	names := []string{"list", "info_update", "load", "cancel", "close", "delete", "resume", "fork"}
	out := make(map[string]middleware.CapabilityDescriptor, len(names))
	for _, name := range names {
		out[name] = middleware.CapabilityDescriptor{
			Name:      name,
			Supported: false,
			Status:    "unsupported",
			Stability: "unsupported",
			Source:    "provider_does_not_expose_session_control",
		}
	}
	return out
}

func capabilityDetailsFromBooleans(caps middleware.ConversationSessionCapabilities) map[string]middleware.CapabilityDescriptor {
	values := map[string]bool{
		"list":        caps.List,
		"info_update": caps.InfoUpdate,
		"load":        caps.Load,
		"cancel":      caps.Cancel,
		"close":       caps.Close,
		"delete":      caps.Delete,
		"resume":      caps.Resume,
		"fork":        caps.Fork,
	}
	out := make(map[string]middleware.CapabilityDescriptor, len(values))
	for name, supported := range values {
		status := "unsupported"
		if supported {
			status = "supported"
		}
		out[name] = middleware.CapabilityDescriptor{
			Name:      name,
			Supported: supported,
			Status:    status,
			Stability: "provider_reported",
			Source:    "conversation_session_capabilities",
		}
	}
	return out
}
