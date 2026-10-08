package agentmgr

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

func isRemoteACPEndpoint(cfg AgentConfig, endpoint middleware.ProtocolEndpoint) bool {
	return endpoint.Kind == middleware.ProtocolKindACP && cfg.Command == "" && endpoint.Address != "" && (endpoint.Transport == "ws" || endpoint.Transport == "http" || endpoint.Transport == "unix")
}

// startWatchdog reserves supervision before spawning so simultaneous registry
// lookups cannot start duplicate children. It uses the daemon lifetime.
func (s *Supervisor) startWatchdog(ctx context.Context, agentID string, cfg AgentConfig) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watching == nil {
		s.watching = make(map[string]chan struct{})
	}
	if ready := s.watching[agentID]; ready != nil {
		return ready
	}
	ready := make(chan struct{})
	s.watching[agentID] = ready
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.watching, agentID)
			if ctx.Err() == nil {
				if s.failed == nil {
					s.failed = make(map[string]bool)
				}
				s.failed[agentID] = true
			}
			s.mu.Unlock()
		}()
		s.watchdog(ctx, agentID, cfg)
	}()
	return ready
}

func (s *Supervisor) ensureSupervision(agentID string, cfg AgentConfig) error {
	s.mu.RLock()
	ctx, process, failed := s.lifetime, s.running[agentID], s.failed[agentID]
	s.mu.RUnlock()
	if process != nil || ctx == nil {
		return nil
	}
	if failed {
		return fmt.Errorf("agent %s supervision gave up; explicit runtime restart required", agentID)
	}
	if !s.proc.HasExecutable(cfg.Command) {
		return fmt.Errorf("agent %s executable not found", agentID)
	}
	ready := s.startWatchdog(ctx, agentID, cfg)
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		slog.Warn("agent supervision is still starting", "agent", agentID)
		return fmt.Errorf("agent %s supervision is still starting", agentID)
	}
}

func (s *Supervisor) startSupervised(ctx context.Context, log *slog.Logger, agentID string, cfg AgentConfig) {
	endpoint := protocolEndpointFromAgentConfig(cfg)
	if isRemoteACPEndpoint(cfg, endpoint) {
		s.startOnDemand(ctx, log, agentID, cfg)
		return
	}
	if !s.proc.HasExecutable(cfg.Command) {
		s.persistRuntimeState(log, RuntimeState{AgentID: agentID, Protocol: string(endpoint.Kind), Mode: runtimeMode(endpoint), Status: "missing_executable", Error: "executable not found in PATH"})
		log.Warn("agent not found in path, skipping supervision", "event", "agent_missing", "agent", agentID, "command", cfg.Command)
		return
	}
	s.startWatchdog(ctx, agentID, cfg)
}
