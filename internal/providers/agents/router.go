package agents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// Router implements middleware.AgentRouter using protocol-specific adapters behind
// a protocol-neutral boundary.
type Router struct {
	resolver middleware.AgentEndpointResolver

	mu      sync.RWMutex
	clients map[string]middleware.ConversationClient
	factory map[middleware.ProtocolKind]middleware.ConversationFactory

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
		resolver: resolver,
		clients:  make(map[string]middleware.ConversationClient),
		factory: map[middleware.ProtocolKind]middleware.ConversationFactory{
			middleware.ProtocolKindACP: &acpConversationFactory{},
			middleware.ProtocolKindA2A: &a2aConversationFactory{},
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
		log.Info("detected dead agent client, pre-warming replacement", "event", "keepalive_reconnect", "agent", agentID, "cwd", cwd)
		if err := r.preWarm(r.keepAliveCtx, agentID, cwd); err != nil {
			log.Warn("keepalive pre-warm failed, will retry on next check", "event", "keepalive_prewarm_failed", "agent", agentID, "cwd", cwd, "error", err)
		}
	}
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
		return nil, "", err
	}
	return client, endpoint.Kind, nil
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
	metadata middleware.ConversationMetadata
}

func (o *simpleObserver) OnUpdate(notif acpSessionNotification) {
	log := slog.With("component", "acp_observer", "session", notif.SessionID, "update_type", notif.Update.SessionUpdate)
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
				Type:     t,
				Content:  notif.Update.Content.Text,
				Title:    notif.Update.Title,
				Metadata: toolUpdateMetadata(notif),
			})
		}
	}

	if notif.Update.Title != "" || notif.Update.UpdatedAt != "" || len(notif.Update.Meta) > 0 {
		o.mu.Lock()
		if notif.Update.Title != "" {
			o.metadata.Title = notif.Update.Title
		}
		if notif.Update.UpdatedAt != "" {
			o.metadata.UpdatedAt = notif.Update.UpdatedAt
		}
		if len(notif.Update.Meta) > 0 {
			if o.metadata.Meta == nil {
				o.metadata.Meta = make(map[string]interface{}, len(notif.Update.Meta))
			}
			for k, v := range notif.Update.Meta {
				o.metadata.Meta[k] = v
			}
		}
		o.mu.Unlock()
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

func (o *simpleObserver) Metadata() middleware.ConversationMetadata {
	o.mu.Lock()
	defer o.mu.Unlock()
	meta := middleware.ConversationMetadata{
		Title:     o.metadata.Title,
		UpdatedAt: o.metadata.UpdatedAt,
		Status:    o.metadata.Status,
	}
	if len(o.metadata.Meta) > 0 {
		meta.Meta = make(map[string]interface{}, len(o.metadata.Meta))
		for k, v := range o.metadata.Meta {
			meta.Meta[k] = v
		}
	}
	return meta
}

func toolUpdateMetadata(notif acpSessionNotification) map[string]interface{} {
	meta := make(map[string]interface{}, len(notif.Update.Meta)+4)
	meta["source_update_type"] = notif.Update.SessionUpdate
	meta["content_type"] = notif.Update.Content.Type
	meta["protocol"] = "acp"
	meta["protocol_method"] = "session/update"
	meta["acp"] = map[string]interface{}{
		"session_id":     notif.SessionID,
		"session_update": notif.Update.SessionUpdate,
		"tool_call_id":   notif.Update.ToolCallID,
		"tool_kind":      notif.Update.Kind,
		"status":         notif.Update.Status,
		"raw_input":      notif.Update.RawInput,
		"locations":      notif.Update.Locations,
		"content": map[string]interface{}{
			"type": notif.Update.Content.Type,
			"text": notif.Update.Content.Text,
		},
		"title":      notif.Update.Title,
		"updated_at": notif.Update.UpdatedAt,
		"_meta":      notif.Update.Meta,
	}
	if strings.TrimSpace(notif.Update.Title) != "" {
		meta["title"] = notif.Update.Title
	}
	if strings.TrimSpace(notif.SessionID) != "" {
		meta["remote_session_id"] = notif.SessionID
	}
	if strings.TrimSpace(notif.Update.ToolCallID) != "" {
		meta["tool_call_id"] = notif.Update.ToolCallID
	}
	if strings.TrimSpace(notif.Update.Kind) != "" {
		meta["tool_kind"] = notif.Update.Kind
		meta["acp_tool_kind"] = notif.Update.Kind
	}
	if strings.TrimSpace(notif.Update.Status) != "" {
		meta["status"] = notif.Update.Status
	}
	if len(notif.Update.RawInput) > 0 {
		meta["raw_input"] = notif.Update.RawInput
	}
	if len(notif.Update.Locations) > 0 {
		meta["locations"] = notif.Update.Locations
	}
	for k, v := range notif.Update.Meta {
		meta[k] = v
	}
	return meta
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
					available = append(available, v.ID)
				}
			}
		}
	}
	if resp.Modes != nil {
		for _, m := range resp.Modes.AvailableModes {
			available = append(available, m.ID)
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
func (r *Router) executePrompt(ctx context.Context, client middleware.ConversationClient, req middleware.RouteRequest) (string, string, []middleware.ToolCall, middleware.ConversationMetadata, error) {
	turn := middleware.ConversationTurn{
		AgentID:          req.AgentID,
		LogicalSessionID: req.LogicalSessionID,
		RemoteSessionID:  req.AgentSessionID,
		WorkspacePath:    req.WorkspacePath,
		Message:          req.Message,
		SidecarCapsules:  req.SidecarCapsules,
		Tools:            req.Tools,
		ThoughtNotifier:  req.ThoughtNotifier,
	}
	result, err := client.ExecuteTurn(ctx, turn)
	if err != nil && turn.RemoteSessionID != "" && isSessionNotFoundError(err) {
		turn.RemoteSessionID = ""
		result, err = client.ExecuteTurn(ctx, turn)
	}
	if err != nil {
		return "", result.RemoteSessionID, result.ToolCalls, result.Metadata, err
	}
	return result.Output, result.RemoteSessionID, result.ToolCalls, result.Metadata, nil
}
