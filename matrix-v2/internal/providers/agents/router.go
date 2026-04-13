package agents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
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

// Router implements middleware.AgentRouter, abstracting the routing of ACP messages
// to dynamic provider runners (WebSockets or Stdio) depending on the
// protocol resolved by the APM logic layer.
type Router struct {
	resolver middleware.AgentEndpointResolver

	mu      sync.RWMutex
	clients map[string]middleware.AgentClient // agentID → persistent ACP client

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

func NewRouter(resolver middleware.AgentEndpointResolver) *Router {
	return &Router{
		resolver: resolver,
		clients:  make(map[string]middleware.AgentClient),
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
		if acp, ok := client.(*ACPClient); ok {
			acp.cancel()
		}
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
		if !isReusableACPClient(client) {
			agentIDs = append(agentIDs, id)
		}
	}
	r.mu.RUnlock()

	if len(agentIDs) == 0 {
		return
	}

	log := slog.With("component", "agent_router_keepalive")
	for _, agentID := range agentIDs {
		log.Info("detected dead agent client, pre-warming replacement", "event", "keepalive_reconnect", "agent", agentID)
		if err := r.preWarm(r.keepAliveCtx, agentID); err != nil {
			log.Warn("keepalive pre-warm failed, will retry on next check", "event", "keepalive_prewarm_failed", "agent", agentID, "error", err)
		}
	}
}

// preWarm evicts a dead client entry and creates a fresh one in its place.
// The new client is fully initialized (initialize handshake complete) so it is
// ready for session/new + prompt on the next user message.
func (r *Router) preWarm(ctx context.Context, agentID string) error {
	log := slog.With("component", "agent_router", "agent", agentID)

	r.mu.Lock()
	// Evict the dead entry
	delete(r.clients, agentID)

	// Create a fresh client under the write lock
	acpClient, protocol, err := r.createClient(ctx, agentID, log)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	r.clients[agentID] = acpClient
	r.mu.Unlock()

	log.Info("keepalive: pre-warmed agent client", "event", "keepalive_prewarmed", "protocol", protocol)
	return nil
}

// Route finds the matching Agent Endpoint, connects a JSON-RPC struct, and executes the Prompt.
func (r *Router) Route(ctx context.Context, req middleware.RouteRequest) (string, string, []middleware.ToolCall, error) {
	client, err := r.getOrCreateClient(ctx, req.AgentID)
	if err != nil {
		return "", "", nil, err
	}
	return r.executePrompt(ctx, client, req)
}

// getOrCreateClient returns a cached client if healthy, or creates a new one.
// The fast path (RLock only) is taken when a reusable client exists.
// The slow path (full Lock) spawns a process, initializes, and caches.
func (r *Router) getOrCreateClient(ctx context.Context, agentID string) (middleware.AgentClient, error) {
	log := slog.With("component", "agent_router", "agent", agentID)

	// Fast path: reusable client exists
	if client, ok := r.lookupReusableClient(agentID); ok {
		log.Debug("reusing cached acp client", "event", "client_reused")
		return client, nil
	}

	// Slow path: create new client under write lock
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have created it)
	if client, ok := r.lookupReusableClientLocked(agentID); ok {
		return client, nil
	}

	// Evict dead entry before creating a new one
	delete(r.clients, agentID)

	acpClient, protocol, err := r.createClient(ctx, agentID, log)
	if err != nil {
		return nil, err
	}
	log.Info("acp client initialized", "event", "client_initialized", "protocol", protocol)
	r.clients[agentID] = acpClient
	return acpClient, nil
}

func (r *Router) lookupReusableClient(agentID string) (middleware.AgentClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lookupReusableClientLocked(agentID)
}

func (r *Router) lookupReusableClientLocked(agentID string) (middleware.AgentClient, bool) {
	client, ok := r.clients[agentID]
	if !ok || !isReusableACPClient(client) {
		return nil, false
	}
	return client, true
}

// isReusableACPClient checks if a cached client is still alive.
// A client is reusable when its internal readLoop goroutine is still running,
// which means the transport (stdio process / WebSocket / unix socket) is alive
// and the derived context has not been cancelled.
func isReusableACPClient(client middleware.AgentClient) bool {
	acpClient, isACP := client.(*ACPClient)
	return isACP && acpClient.ctx.Err() == nil
}

func (r *Router) createClient(ctx context.Context, agentID string, log *slog.Logger) (*ACPClient, string, error) {
	protocol, address, args, env, err := r.resolver.GetAgentEndpoint(agentID)
	if err != nil {
		return nil, "", fmt.Errorf("router failed to resolve endpoint for agent %s: %w", agentID, err)
	}
	log.Info("resolved agent endpoint", "event", "endpoint_resolved", "protocol", protocol, "address", address, "args_count", len(args), "env_count", len(env))

	transport, err := createTransport(ctx, transportSpec{
		Protocol: protocol,
		Address:  address,
		Args:     args,
		Env:      env,
	})
	if err != nil {
		return nil, "", err
	}

	acpClient := NewACPClient(ctx, transport)
	handler := newConfigurableRequestHandler(r.trustMode).WithFS(r.fs, r.cwd)
	if r.proc != nil {
		handler.WithProcess(r.proc)
	}
	acpClient.SetRequestHandler(handler)
	initReq := middleware.InitializeRequest{
		ProtocolVersion: 1,
		ClientInfo:      map[string]interface{}{"name": "matrix", "version": "1.0"},
		ClientCapabilities: &middleware.ClientCapabilities{
			Fs: &middleware.FsCapability{
				ReadTextFile:  r.fs != nil,
				WriteTextFile: r.fs != nil,
			},
			Terminal: r.proc != nil,
		},
	}
	initResp, err := acpClient.Initialize(ctx, initReq)
	if err != nil {
		_ = transport.Close()
		return nil, "", fmt.Errorf("ACP initialize failed: %w", err)
	}
	if len(initResp.AuthMethods) > 0 {
		log.Info("agent requires authentication",
			"event", "auth_required",
			"agent", agentID,
			"methods", len(initResp.AuthMethods),
		)
		for _, m := range initResp.AuthMethods {
			log.Debug("auth method", "type", m.Type, "id", m.ID, "description", m.Description)
			if m.Type == "env_var" && m.EnvVar != "" {
				if val, ok := os.LookupEnv(m.EnvVar); ok {
					log.Info("auto-authenticating with env var", "envVar", m.EnvVar, "methodId", m.ID)
					creds := map[string]string{"api_key": val}
					if err := acpClient.Authenticate(ctx, m.ID, creds); err != nil {
						log.Warn("authentication failed", "methodId", m.ID, "error", err)
					} else {
						log.Info("authentication succeeded", "methodId", m.ID)
					}
				} else {
					log.Warn("auth env var not set", "envVar", m.EnvVar)
				}
			}
		}
	}
	return acpClient, protocol, nil
}

type transportSpec struct {
	Protocol string
	Address  string
	Args     []string
	Env      []string
}

func createTransport(ctx context.Context, spec transportSpec) (middleware.AgentTransport, error) {
	switch spec.Protocol {
	case "ws":
		addr := spec.Address
		if !strings.HasPrefix(addr, "ws://") && !strings.HasPrefix(addr, "wss://") {
			addr = "ws://" + addr
		}
		return NewWSTransport(ctx, addr)
	case "stdio":
		return NewStdioTransport(ctx, spec.Address, spec.Env, spec.Args...)
	case "unix":
		return NewUnixTransport(ctx, spec.Address)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", spec.Protocol)
	}
}

// ----------------------------------------------------------------------------
// Observer — buffers streaming text chunks into a final response
// ----------------------------------------------------------------------------

// simpleObserver buffers text chunks to form the final result.
// It also forwards real-time thought/tool updates to an optional ThoughtNotifier
// so the UI (e.g. Telegram) can show a live "thinking" indicator.
type simpleObserver struct {
	mu       sync.Mutex
	content  string
	updates  chan struct{}
	notifier middleware.ThoughtNotifier
}

func (o *simpleObserver) OnUpdate(notif middleware.SessionNotification) {
	log := slog.With("component", "acp_observer", "session", notif.SessionId, "update_type", notif.Update.SessionUpdate)
	log.Info("session update received", "event", "session_update", "update_type", notif.Update.SessionUpdate, "text_len", len(notif.Update.Content.Text), "text_preview", truncate(notif.Update.Content.Text, 120))

	switch notif.Update.SessionUpdate {
	case "agent_message_chunk":
		o.mu.Lock()
		o.content += notif.Update.Content.Text
		o.mu.Unlock()
		// Stream message chunks to the thought notifier in real-time
		if o.notifier != nil && notif.Update.Content.Text != "" {
			o.notifier.OnThought(middleware.ThoughtUpdate{
				Type:    middleware.ThoughtTypeThinking,
				Content: notif.Update.Content.Text,
			})
		}
		if o.updates != nil {
			select {
			case o.updates <- struct{}{}:
			default:
			}
		}
	case "agent_thought_chunk":
		// Agent reasoning — not included in the user-visible response.
		// Forwarded to the UI as a "thinking" indicator if a notifier is set.
		if o.notifier != nil {
			o.notifier.OnThought(middleware.ThoughtUpdate{
				Type:    middleware.ThoughtTypeThinking,
				Content: notif.Update.Content.Text,
			})
		}
	case "tool_call", "tool_call_update":
		log.Info("tool call update", "event", "tool_call_update", "text_len", len(notif.Update.Content.Text))
		if o.notifier != nil {
			t := middleware.ThoughtTypeToolCall
			if notif.Update.SessionUpdate == "tool_call_update" {
				t = middleware.ThoughtTypeToolResult
			}
			o.notifier.OnThought(middleware.ThoughtUpdate{
				Type:    t,
				Content: notif.Update.Content.Text,
			})
		}
	}
}

func (o *simpleObserver) GetContent() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return stripThinking(o.content)
}

// RawContent returns the unfiltered content (including think blocks) for debugging.
func (o *simpleObserver) RawContent() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.content
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// stripThinking removes <think...</think...> blocks from agent output.
// Some agents emit reasoning inside these tags; they should not reach the user.
func stripThinking(s string) string {
	return stripTagBlock(s, "<think", "</think")
}

func stripTagBlock(s, openTag, closeTag string) string {
	for {
		start := strings.Index(s, openTag)
		if start == -1 {
			break
		}
		// Find end of the opening tag (skip attributes like <think xmlns=...>)
		tagEnd := strings.Index(s[start:], ">")
		if tagEnd == -1 {
			break
		}
		end := strings.Index(s[start:], closeTag)
		if end == -1 {
			break
		}
		closeEnd := strings.Index(s[start+end:], ">")
		if closeEnd == -1 {
			break
		}
		s = s[:start] + s[start+end+closeEnd+1:]
	}
	return s
}

// WaitIdle blocks until the stream has been silent for the given duration,
// indicating the agent has finished emitting chunks.
func (o *simpleObserver) WaitIdle(ctx context.Context, idle time.Duration) {
	if o.updates == nil {
		return
	}

	timer := time.NewTimer(idle)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			return
		case <-o.updates:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idle)
		}
	}
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

func (r *Router) createSession(ctx context.Context, client middleware.AgentClient, logicalSessionID string, tools []middleware.Tool) (string, error) {
	log := slog.With("component", "agent_router", "logical_session", logicalSessionID)
	newSessReq := middleware.NewSessionRequest{
		ClientTitle: logicalSessionID,
		Cwd:         r.cwd,
		McpServers:  []middleware.McpServerConfig{},
		Tools:       tools,
	}
	newSessResp, err := client.NewSession(ctx, newSessReq)
	if err != nil {
		return "", fmt.Errorf("ACP new session failed: %w", err)
	}
	log.Info("created acp session", "event", "session_created", "agent_session", newSessResp.SessionId, "tools_count", len(tools))

	// Try to set the most permissive mode available.
	// Mode IDs are agent-defined (e.g. "code", "yolo", "autoEdit").
	// We look for known auto-approve modes, falling back to the first available.
	if modeID := pickAutoApproveMode(newSessResp); modeID != "" {
		if err := client.SetMode(ctx, newSessResp.SessionId, modeID); err != nil {
			log.Warn("failed to set agent mode, continuing anyway", "event", "mode_set_failed", "modeId", modeID, "error", err)
		} else {
			log.Info("set agent mode", "event", "mode_set", "agent_session", newSessResp.SessionId, "modeId", modeID)
		}
	}

	return newSessResp.SessionId, nil
}

// pickAutoApproveMode finds the most permissive mode from the session/new response.
// Priority: build > yolo > auto > autoEdit > code > agent > first available.
// Mode IDs are agent-defined strings, so we match case-insensitively against
// known write-enabled mode names. "build" is opencode's write mode, "yolo" is
// Zed's auto-approve mode, "code" is Claude Code's default mode.
func pickAutoApproveMode(resp *middleware.NewSessionResponse) string {
	var available []string
	if resp.ConfigOptions != nil {
		for _, opt := range resp.ConfigOptions {
			if opt.Category == "mode" {
				for _, v := range opt.Options {
					available = append(available, v.Id)
				}
			}
		}
	}
	if resp.Modes != nil {
		for _, m := range resp.Modes.AvailableModes {
			available = append(available, m.Id)
		}
	}
	if len(available) == 0 {
		return ""
	}

	// Priority order for known auto-approve/write-enabled mode IDs
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
func (r *Router) executePrompt(ctx context.Context, client middleware.AgentClient, req middleware.RouteRequest) (string, string, []middleware.ToolCall, error) {
	log := slog.With("component", "agent_router", "logical_session", req.LogicalSessionID)

	// Create session only if we do not have an active agentSessionID
	if req.AgentSessionID == "" {
		var err error
		req.AgentSessionID, err = r.createSession(ctx, client, req.LogicalSessionID, req.Tools)
		if err != nil {
			return "", "", nil, err
		}
	}
	log.Debug("dispatching acp prompt", "event", "prompt_dispatch", "agent_session", req.AgentSessionID, "message_len", len(req.Message))

	// Notify the thought messenger about agent/session identity for UI display
	if req.ThoughtNotifier != nil {
		log.Info("calling SetHeader on thought notifier", "event", "setheader_call", "agentID", req.AgentID, "agentSessionID", req.AgentSessionID)
		req.ThoughtNotifier.SetHeader(req.AgentID, req.AgentSessionID)
	}

	// Execute the prompt with a streaming observer
	obs := &simpleObserver{updates: make(chan struct{}, 1), notifier: req.ThoughtNotifier}
	promptReq := middleware.PromptRequest{
		SessionId: req.AgentSessionID,
		Prompt: []middleware.Content{
			{Type: "text", Text: req.Message},
		},
	}

	resp, err := client.Prompt(ctx, promptReq, obs)
	if err != nil {
		// If the session was lost (agent restarted), create a new one and retry
		if req.AgentSessionID != "" && isSessionNotFoundError(err) {
			log.Warn("acp session missing, creating a new session and retrying prompt", "event", "prompt_retry_new_session", "stale_agent_session", req.AgentSessionID)
			req.AgentSessionID, err = r.createSession(ctx, client, req.LogicalSessionID, req.Tools)
			if err != nil {
				return "", "", nil, err
			}
			obs = &simpleObserver{updates: make(chan struct{}, 1), notifier: req.ThoughtNotifier}
			promptReq.SessionId = req.AgentSessionID
			log.Debug("retrying acp prompt", "event", "prompt_retry", "agent_session", req.AgentSessionID, "message_len", len(req.Message))
			resp, err = client.Prompt(ctx, promptReq, obs)
		}
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("ACP prompt failed: %w", err)
	}

	// Wait until the stream becomes idle instead of sleeping blindly.
	obs.WaitIdle(ctx, 150*time.Millisecond)
	cleanContent := obs.GetContent()
	log.Info("acp prompt completed", "event", "prompt_completed", "agent_session", req.AgentSessionID, "response_len", len(cleanContent), "tool_calls", len(resp.ToolCalls), "stop_reason", resp.StopReason)
	log.Debug("acp response detail", "event", "response_detail", "agent_session", req.AgentSessionID, "raw_len", len(obs.RawContent()), "clean_len", len(cleanContent))

	return cleanContent, req.AgentSessionID, resp.ToolCalls, nil
}
