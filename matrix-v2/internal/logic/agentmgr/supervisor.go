package agentmgr

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

const (
	maxFastCrashes  = 5
	fastCrashWindow = 5 * time.Second
)

// AgentProcess tracks a running agent child process
type AgentProcess struct {
	AgentID string
	Port    int
	Handle  middleware.ProcessHandle
}

type supervisedRun struct {
	AgentID   string
	Handle    middleware.ProcessHandle
	StartedAt time.Time
}

// Supervisor manages the lifecycle, routing ports, and SSOT persistence of agent CLIs.
type Supervisor struct {
	proc     middleware.Process
	net      middleware.Network
	store    middleware.Storage
	registry *Registry

	mu      sync.RWMutex
	running map[string]*AgentProcess
}

// NewSupervisor instantiates the APM.
func NewSupervisor(proc middleware.Process, netprov middleware.Network, store middleware.Storage, reg *Registry) *Supervisor {
	return &Supervisor{
		proc:     proc,
		net:      netprov,
		store:    store,
		registry: reg,
		running:  make(map[string]*AgentProcess),
	}
}

// StartAll reads the registry and starts all installed/enabled agents.
func (s *Supervisor) StartAll(ctx context.Context) error {
	log := slog.With("component", "agent_supervisor")

	for _, agentID := range s.registry.List() {
		cfg, err := s.registry.Get(agentID)
		if err != nil {
			log.Warn("agent missing from registry during start-all", "event", "registry_lookup_failed", "agent", agentID, "error", err)
			continue
		}

		// 1. Is it installed? (Graceful Degradation)
		if !s.proc.HasExecutable(cfg.Command) {
			s.persistRuntimeState(log, RuntimeState{AgentID: agentID, Protocol: cfg.Protocol, Mode: runtimeMode(cfg.Protocol), Status: "missing_executable", Error: "executable not found in PATH"})
			log.Warn("agent not found in path, skipping supervision", "event", "agent_missing", "agent", agentID, "command", cfg.Command)
			continue
		}

		// 2. Start supervision loop ONLY for networked agents (ws, http)
		// stdio agents are handled on-demand by the Router.
		if cfg.Protocol == "ws" || cfg.Protocol == "http" {
			go s.watchdog(ctx, agentID, cfg)
		} else {
			s.persistRuntimeState(log, RuntimeState{AgentID: agentID, Protocol: cfg.Protocol, Mode: runtimeMode(cfg.Protocol), Status: "ready_on_demand"})
			log.Info("agent is on-demand, skipping background supervision", "event", "agent_on_demand", "agent", agentID, "protocol", cfg.Protocol)
		}
	}
	return nil
}

// GetAgentEndpoint returns the assigned protocol, address, arguments and environment for the agent.
func (s *Supervisor) GetAgentEndpoint(agentID string) (string, string, []string, []string, error) {
	cfg, err := s.registry.Get(agentID)
	if err != nil {
		return "", "", nil, nil, fmt.Errorf("agent %s configuration not found in registry: %w", agentID, err)
	}
	// On-demand agents are spawned by the Router via stdio — skip running-map lookup.
	// "acp" is the protocol assigned by the registry installer; it speaks ACP over stdio.
	if cfg.Protocol == "stdio" || cfg.Protocol == "acp" {
		return "stdio", cfg.Command, cfg.Args, cfg.Env, nil
	}

	if !cfg.IsActive() {
		return "", "", nil, nil, fmt.Errorf("agent %s is configured but inactive", agentID)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.running[agentID]
	if !ok {
		return "", "", nil, nil, fmt.Errorf("agent %s is not currently running or tracked by APM", agentID)
	}

	return cfg.Protocol, fmt.Sprintf("127.0.0.1:%d", p.Port), nil, cfg.Env, nil
}

// watchdog keeps an agent alive. If it dies, it restarts with backoff.
// If the agent crashes too quickly too many times (crash-loop), it gives up.
func (s *Supervisor) watchdog(ctx context.Context, agentID string, cfg AgentConfig) {
	log := slog.With("component", "agent_supervisor", "agent", agentID)

	consecutiveFastCrashes := 0

	for {
		select {
		case <-ctx.Done():
			log.Info("shutting down agent supervisor", "event", "supervisor_stopped")
			return
		default:
		}

		port, ok := s.allocatePort(log)
		if !ok {
			continue
		}

		args := injectPortArgs(cfg.Args, port)
		log.Info("starting supervised agent", "event", "agent_starting", "port", port, "command", cfg.Command, "args", args)
		spec := middleware.CommandSpec{Runner: cfg.Command, Args: args, Env: cfg.Env, EnvIsolation: cfg.EnvIsolation}
		s.persistRuntimeState(log, RuntimeState{AgentID: agentID, Protocol: cfg.Protocol, Mode: runtimeMode(cfg.Protocol), Status: "starting", Port: port, Address: fmt.Sprintf("127.0.0.1:%d", port)})

		run, ok := s.startAgent(log, agentID, port, spec)
		if !ok {
			continue
		}
		if !s.handleExit(log, run, &consecutiveFastCrashes) {
			return
		}
	}
}

func (s *Supervisor) allocatePort(log *slog.Logger) (int, bool) {
	port, err := s.net.GetFreePort()
	if err != nil {
		log.Warn("failed to get free port for agent, retrying", "event", "port_allocation_failed", "error", err, "retry_in", "5s")
		time.Sleep(5 * time.Second)
		return 0, false
	}
	return port, true
}

func injectPortArgs(args []string, port int) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = strings.ReplaceAll(arg, "{PORT}", fmt.Sprintf("%d", port))
	}
	return out
}

func (s *Supervisor) startAgent(log *slog.Logger, agentID string, port int, spec middleware.CommandSpec) (supervisedRun, bool) {
	startedAt := time.Now()
	handle, err := s.proc.Start(spec)
	if err != nil {
		s.persistRuntimeState(log, RuntimeState{AgentID: agentID, Protocol: "unknown", Mode: "supervised", Status: "start_failed", Error: err.Error()})
		log.Warn("failed to start supervised agent, retrying", "event", "agent_start_failed", "error", err, "retry_in", "5s")
		time.Sleep(5 * time.Second)
		return supervisedRun{}, false
	}

	s.mu.Lock()
	s.running[agentID] = &AgentProcess{AgentID: agentID, Port: port, Handle: handle}
	s.mu.Unlock()
	s.persistRuntimeState(log, RuntimeState{AgentID: agentID, Protocol: "ws", Mode: "supervised", Status: "running", Port: port, Address: fmt.Sprintf("127.0.0.1:%d", port), PID: handle.GetPID()})
	return supervisedRun{AgentID: agentID, Handle: handle, StartedAt: startedAt}, true
}

func (s *Supervisor) handleExit(log *slog.Logger, run supervisedRun, consecutiveFastCrashes *int) bool {
	err := run.Handle.Wait()
	elapsed := time.Since(run.StartedAt)
	log.Warn("supervised agent exited", "event", "agent_exited", "elapsed", elapsed.Round(time.Millisecond), "error", err)

	s.unregisterRunning(run.AgentID)

	if elapsed >= fastCrashWindow {
		s.persistRuntimeState(log, RuntimeState{AgentID: run.AgentID, Mode: "supervised", Status: "stopped", Error: errorString(err)})
		*consecutiveFastCrashes = 0
		time.Sleep(2 * time.Second)
		return true
	}

	*consecutiveFastCrashes++
	if *consecutiveFastCrashes >= maxFastCrashes {
		s.persistRuntimeState(log, RuntimeState{AgentID: run.AgentID, Mode: "supervised", Status: "crash_loop", Error: errorString(err)})
		log.Error("agent entered crash loop, giving up", "event", "agent_crash_loop", "crashes", maxFastCrashes, "window", fastCrashWindow)
		return false
	}

	s.persistRuntimeState(log, RuntimeState{AgentID: run.AgentID, Mode: "supervised", Status: "fast_crash", Error: errorString(err)})
	log.Warn("agent fast-crash detected, delaying restart", "event", "agent_fast_crash", "count", *consecutiveFastCrashes, "max", maxFastCrashes, "retry_in", "3s")
	time.Sleep(3 * time.Second)
	return true
}

func (s *Supervisor) unregisterRunning(agentID string) {
	s.mu.Lock()
	delete(s.running, agentID)
	s.mu.Unlock()
}

func (s *Supervisor) persistRuntimeState(log *slog.Logger, state RuntimeState) {
	if state.AgentID == "" {
		return
	}
	if err := SaveRuntimeState(s.store, state); err != nil {
		log.Warn("failed to persist runtime state", "event", "runtime_state_persist_failed", "agent", state.AgentID, "error", err)
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
