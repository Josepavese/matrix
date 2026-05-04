package agents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/a2aclient"
)

// ----------------------------------------------------------------------------
// Router — persistent ACP client pool with proactive health checking
//
// Architecture:
//
//   Matrix (this router) maintains a pool of ACPClient instances keyed by
//   agentID. Each client wraps a long-lived transport (stdio process, WebSocket,
//   unix socket). The ACP spec models agents as long-running sub-processes (like
//   LSP servers) that stay alive between prompt turns.
//
//   To avoid cold-start latency (spawn + initialize + session/new ≈ 10-30s plus
//   LLM warm-up), the router keeps clients alive across messages. A background
//   health-check goroutine detects dead clients and proactively reconnects so
//   the next user message finds a warm, ready client.
//
//   Lifecycle:
//     1. First message for agentID → spawn process, initialize, cache client
//     2. Subsequent messages → reuse cached client (no cold start)
//     3. If transport dies (EOF, crash) → readLoop exits, client marked dead
//     4. Health checker detects dead client → evicts and pre-warms a new one
//     5. Next message arrives → warm client ready, ~20s instead of ~2min
//
// ----------------------------------------------------------------------------

// healthCheckInterval is how often the background goroutine scans for dead clients.
const healthCheckInterval = 30 * time.Second
const clientReapTombstoneTTL = 5 * time.Minute

// Router implements middleware.AgentRouter using protocol-specific adapters behind
// a protocol-neutral boundary.
type Router struct {
	resolver middleware.AgentEndpointResolver

	mu      sync.RWMutex
	clients map[string]middleware.ConversationClient
	factory map[middleware.ProtocolKind]middleware.ConversationFactory

	// Tombstones preserve short-lived OS/process cleanup proof when a dead
	// stdio client is evicted by keepalive before strict session cleanup runs.
	clientTombstones map[string]agentClientTombstone

	// trustMode returns true when auto-approve is enabled (default).
	// When false, permission requests from agents are denied.
	// If nil, defaults to true (trust mode on).
	trustMode func() bool

	// fs and cwd enable fs/* ACP methods (agent reads/writes files via protocol).
	fs  middleware.FS
	cwd string

	// proc enables terminal/* ACP methods (agent executes commands).
	proc middleware.Process

	// keepaliveCtx/keepaliveCancel control the background health-check goroutine.
	// The context is derived from the one passed to StartKeepalive, which should
	// be the daemon's top-level signal context (lives until SIGINT/SIGTERM).
	keepAliveCtx    context.Context
	keepAliveCancel context.CancelFunc
}

// NewRouter creates a new agent Router with the given endpoint resolver.
func NewRouter(resolver middleware.AgentEndpointResolver) *Router {
	return &Router{
		resolver:         resolver,
		clients:          make(map[string]middleware.ConversationClient),
		clientTombstones: make(map[string]agentClientTombstone),
		factory: map[middleware.ProtocolKind]middleware.ConversationFactory{
			middleware.ProtocolKindACP: &acpConversationFactory{},
			middleware.ProtocolKindA2A: a2aclient.Factory{},
		},
	}
}

// SetTrustMode sets the function that determines whether agent permission
// requests are auto-approved. Pass nil to always auto-approve (default).
func (r *Router) SetTrustMode(fn func() bool) {
	r.trustMode = fn
}

// SetFS configures filesystem access for fs/* ACP methods.
func (r *Router) SetFS(fs middleware.FS, cwd string) {
	r.fs = fs
	r.cwd = cwd
}

// SetProcess configures process execution for terminal/* ACP methods.
func (r *Router) SetProcess(proc middleware.Process) {
	r.proc = proc
}

// StartKeepalive launches the background health-check goroutine.
// Call this once at daemon startup with the daemon's long-lived signal context.
// The goroutine will stop when ctx is cancelled or StopKeepalive is called.
func (r *Router) StartKeepalive(ctx context.Context) {
	r.mu.Lock()
	r.keepAliveCtx, r.keepAliveCancel = context.WithCancel(ctx)
	r.mu.Unlock()

	go r.keepaliveLoop()
	slog.Info("agent router keepalive started", "event", "keepalive_started", "interval", healthCheckInterval)
}

// StopKeepalive terminates the background health-check goroutine.
func (r *Router) StopKeepalive() {
	r.mu.Lock()
	if r.keepAliveCancel != nil {
		r.keepAliveCancel()
	}
	r.mu.Unlock()
	slog.Info("agent router keepalive stopped", "event", "keepalive_stopped")
}

// Close terminates all cached clients and stops the health checker.
// Call during daemon shutdown for clean resource release.
func (r *Router) Close() {
	r.StopKeepalive()
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, client := range r.clients {
		_ = client.Close()
		delete(r.clients, id)
	}
}

// keepaliveLoop is the background goroutine that periodically checks client health
// and proactively reconnects dead clients so the next user message finds a warm client.
func (r *Router) keepaliveLoop() {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.keepAliveCtx.Done():
			return
		case <-ticker.C:
			r.checkAndReconnect()
		}
	}
}

// checkAndReconnect scans the client pool for dead entries.
// When a dead client is found, it is evicted and a new one is pre-warmed
// so the next user message does not pay the cold-start penalty.
func (r *Router) checkAndReconnect() {
	r.mu.RLock()
	agentIDs := make([]string, 0, len(r.clients))
	for id, client := range r.clients {
		if !isReusableClient(client) {
			agentIDs = append(agentIDs, id)
		}
	}
	r.mu.RUnlock()

	if len(agentIDs) == 0 {
		return
	}

	log := slog.With("component", "agent_router_keepalive")
	for _, key := range agentIDs {
		agentID, cwd := splitClientCacheKey(key)
		if !r.canSpawnFreshLifecycleClient(agentID) {
			if r.evictCachedClient(key) {
				log.Info("detected dead local agent client, evicted without pre-warm", "event", "keepalive_evict_dead_local_client", "agent", agentID, "cwd", cwd)
			}
			continue
		}
		log.Info("detected dead agent client, pre-warming replacement", "event", "keepalive_reconnect", "agent", agentID, "cwd", cwd)
		if err := r.preWarm(r.keepAliveCtx, agentID, cwd); err != nil {
			log.Warn("keepalive pre-warm failed, will retry on next check", "event", "keepalive_prewarm_failed", "agent", agentID, "cwd", cwd, "error", err)
		}
	}
}

func (r *Router) evictCachedClient(key string) bool {
	r.mu.Lock()
	client, ok := r.clients[key]
	if ok {
		delete(r.clients, key)
		r.rememberClientTombstoneLocked(key, client)
	}
	r.mu.Unlock()
	if ok {
		_ = client.Close()
	}
	return ok
}

// preWarm evicts a dead client entry and creates a fresh one in its place.
// The new client is fully initialized (initialize handshake complete) so it is
// ready for session/new + prompt on the next user message.
func (r *Router) preWarm(ctx context.Context, agentID string, cwd string) error {
	log := slog.With("component", "agent_router", "agent", agentID)
	key := clientCacheKey(agentID, cwd)

	r.mu.Lock()
	// Evict the dead entry
	delete(r.clients, key)

	// Create a fresh client under the write lock
	client, kind, err := r.createClient(ctx, agentID, cwd, log)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	r.clients[key] = client
	r.mu.Unlock()

	log.Info("keepalive: pre-warmed agent client", "event", "keepalive_prewarmed", "protocol_kind", kind)
	return nil
}

// Route finds the matching Agent Endpoint, connects a JSON-RPC struct, and executes the Prompt.
func (r *Router) Route(ctx context.Context, req middleware.RouteRequest) (string, string, []middleware.ToolCall, middleware.ConversationMetadata, error) {
	client, err := r.getOrCreateClient(ctx, req.AgentID, r.effectiveCwd(req.WorkspacePath))
	if err != nil {
		return "", "", nil, middleware.ConversationMetadata{}, err
	}
	return r.executePrompt(ctx, client, req)
}

func (r *Router) createClient(ctx context.Context, agentID string, cwd string, log *slog.Logger) (middleware.ConversationClient, middleware.ProtocolKind, error) {
	endpoint, err := r.resolver.GetAgentEndpoint(agentID)
	if err != nil {
		return nil, "", fmt.Errorf("router failed to resolve endpoint for agent %s: %w", agentID, err)
	}
	log.Info("resolved agent endpoint", "event", "endpoint_resolved", "protocol_kind", endpoint.Kind, "transport", endpoint.Transport, "address", endpoint.Address, "command", endpoint.Command)

	factory, ok := r.factory[endpoint.Kind]
	if !ok {
		return nil, "", fmt.Errorf("unsupported protocol kind: %s", endpoint.Kind)
	}
	client, err := factory.NewClient(ctx, endpoint, middleware.ConversationFactoryDeps{
		FS:        r.fs,
		Cwd:       cwd,
		Process:   r.proc,
		TrustMode: r.trustMode,
	})
	if err != nil {
		return nil, "", annotateProviderFailureAgent(err, agentID)
	}
	return client, endpoint.Kind, nil
}

// ----------------------------------------------------------------------------
// Session management
// ----------------------------------------------------------------------------

func isSessionNotFoundError(err error) bool {
	if errors.Is(err, middleware.ErrSessionNotFound) {
		return true
	}
	// Fallback for agents that return plain text errors
	return strings.Contains(strings.ToLower(err.Error()), "session not found")
}

// pickAutoApproveConfigOption finds the most permissive config-option value
// when the agent exposes ACP's preferred session config options surface.
func pickAutoApproveConfigOption(resp *middleware.NewSessionResponse) (string, string) {
	if resp == nil || len(resp.ConfigOptions) == 0 {
		return "", ""
	}
	for _, opt := range resp.ConfigOptions {
		if opt.Category != "mode" && !strings.EqualFold(opt.ID, "mode") {
			continue
		}
		if value := pickPreferredID(configOptionIDs(opt.Options)); value != "" {
			return opt.ID, value
		}
	}
	return "", ""
}

func configOptionIDs(values []middleware.ConfigOptionValue) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.ID)
	}
	return out
}

// pickAutoApproveMode finds the most permissive legacy mode from the session/new response.
// Priority: build > yolo > auto > autoEdit > code > agent > first available.
// Mode IDs are agent-defined strings, so we match case-insensitively against
// known write-enabled mode names. "build" is opencode's write mode, "yolo" is
// Zed's auto-approve mode, "code" is Claude Code's default mode.
func pickAutoApproveMode(resp *middleware.NewSessionResponse) string {
	var available []string
	if resp.Modes != nil {
		for _, m := range resp.Modes.AvailableModes {
			available = append(available, m.ID)
		}
	}
	if len(available) == 0 {
		return ""
	}

	return pickPreferredID(available)
}

func pickPreferredID(available []string) string {
	if len(available) == 0 {
		return ""
	}
	priority := []string{"build", "yolo", "auto", "autoEdit", "code", "agent"}
	for _, preferred := range priority {
		for _, id := range available {
			if strings.EqualFold(id, preferred) {
				return id
			}
		}
	}
	// Fallback: use the first mode (typically the default write-enabled agent)
	return available[0]
}

// ----------------------------------------------------------------------------
// Prompt execution
// ----------------------------------------------------------------------------

// executePrompt handles the standard ACP flow:
//  1. Create session (if not already existing)
//  2. Set agent mode to most permissive available
//  3. Dispatch prompt with streaming observer
//  4. Collect final response and return
func (r *Router) executePrompt(ctx context.Context, client middleware.ConversationClient, req middleware.RouteRequest) (string, string, []middleware.ToolCall, middleware.ConversationMetadata, error) {
	turn := middleware.ConversationTurn{
		AgentID:           req.AgentID,
		LogicalSessionID:  req.LogicalSessionID,
		RemoteSessionID:   req.AgentSessionID,
		WorkspacePath:     req.WorkspacePath,
		Message:           req.Message,
		SidecarCapsules:   req.SidecarCapsules,
		Tools:             req.Tools,
		ThoughtNotifier:   req.ThoughtNotifier,
		LiveContextAttach: req.LiveContextAttach,
	}
	result, err := client.ExecuteTurn(ctx, turn)
	if err != nil && turn.RemoteSessionID != "" && isSessionNotFoundError(err) && !req.StrictSession {
		turn.RemoteSessionID = ""
		result, err = client.ExecuteTurn(ctx, turn)
	}
	if err != nil {
		return "", result.RemoteSessionID, result.ToolCalls, result.Metadata, err
	}
	return result.Output, result.RemoteSessionID, result.ToolCalls, result.Metadata, nil
}
