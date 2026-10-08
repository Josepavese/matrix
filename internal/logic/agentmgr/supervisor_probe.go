package agentmgr

import (
	"context"
	"github.com/Josepavese/matrix/internal/middleware"
	"log/slog"
)

func (s *Supervisor) probeOnDemand(ctx context.Context, log *slog.Logger, agentID string, endpoint middleware.ProtocolEndpoint) {
	probeCtx, cancel := context.WithTimeout(ctx, onDemandProbeTimeout)
	err := s.probe(probeCtx, endpoint)
	cancel()
	state := RuntimeState{AgentID: agentID, Protocol: string(endpoint.Kind), Mode: runtimeMode(endpoint), Status: "ready_on_demand"}
	if err != nil {
		state.Status = "initialize_failed"
		state.Error = err.Error()
		s.persistRuntimeState(log, state)
		log.Warn("agent initialize probe failed", "event", "agent_initialize_failed", "agent", agentID, "error", err)
		return
	}
	s.persistRuntimeState(log, state)
	log.Info("agent initialize probe passed", "event", "agent_initialize_ready", "agent", agentID)
}
