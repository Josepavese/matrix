package agents

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/jose/matrix-v2/internal/middleware"
)

// ReapAgentClient closes and evicts the cached client for the exact agent/workspace
// binding used by an ephemeral run. For stdio ACP clients this terminates the
// underlying child process through the transport Close path.
func (r *Router) ReapAgentClient(_ context.Context, agentID string, workspacePath string) (bool, error) {
	key := clientCacheKey(agentID, r.effectiveCwd(workspacePath))
	r.mu.Lock()
	client, ok := r.clients[key]
	if ok {
		delete(r.clients, key)
	}
	r.mu.Unlock()
	if !ok {
		return false, nil
	}
	return true, client.Close()
}

func (r *Router) ListAgentSessions(ctx context.Context, agentID string) ([]middleware.RemoteSessionInfo, middleware.ConversationSessionCapabilities, error) {
	client, err := r.getOrCreateSessionControlClient(ctx, agentID)
	if err != nil {
		return nil, middleware.ConversationSessionCapabilities{}, err
	}
	controller, ok := client.(middleware.ConversationSessionControl)
	if !ok {
		return nil, middleware.ConversationSessionCapabilities{}, fmt.Errorf("agent %s does not expose remote session control", agentID)
	}
	sessions, err := controller.ListRemoteSessions(ctx)
	if err != nil {
		return nil, middleware.ConversationSessionCapabilities{}, err
	}
	return sessions, controller.SessionCapabilities(), nil
}

func (r *Router) GetAgentSession(ctx context.Context, agentID string, remoteSessionID string) (middleware.RemoteSessionInfo, error) {
	client, err := r.getOrCreateSessionControlClient(ctx, agentID)
	if err != nil {
		return middleware.RemoteSessionInfo{}, err
	}
	controller, ok := client.(middleware.ConversationSessionControl)
	if !ok {
		return middleware.RemoteSessionInfo{}, fmt.Errorf("agent %s does not expose remote session control", agentID)
	}
	return controller.GetRemoteSession(ctx, remoteSessionID)
}

func (r *Router) CancelAgentSession(ctx context.Context, agentID string, remoteSessionID string) error {
	client, err := r.getOrCreateSessionControlClient(ctx, agentID)
	if err != nil {
		return err
	}
	controller, ok := client.(middleware.ConversationSessionControl)
	if !ok {
		return fmt.Errorf("agent %s does not expose remote session control", agentID)
	}
	return controller.CancelRemoteSession(ctx, remoteSessionID)
}

func (r *Router) CloseAgentSession(ctx context.Context, agentID string, remoteSessionID string) error {
	client, err := r.getOrCreateSessionControlClient(ctx, agentID)
	if err != nil {
		return err
	}
	controller, ok := client.(middleware.ConversationSessionControl)
	if !ok || !controller.SessionCapabilities().Close {
		return fmt.Errorf("agent %s does not expose remote session close", agentID)
	}
	return controller.CloseRemoteSession(ctx, remoteSessionID)
}

func (r *Router) CancelAgentSessionForWorkspace(ctx context.Context, agentID string, remoteSessionID string, workspacePath string) error {
	client, err := r.getOrCreateSessionControlClientForWorkspace(ctx, agentID, workspacePath)
	if err != nil {
		return err
	}
	controller, ok := client.(middleware.ConversationSessionControl)
	if !ok || !controller.SessionCapabilities().Cancel {
		return fmt.Errorf("agent %s does not expose remote session control", agentID)
	}
	return controller.CancelRemoteSession(ctx, remoteSessionID)
}

func (r *Router) CloseAgentSessionForWorkspace(ctx context.Context, agentID string, remoteSessionID string, workspacePath string) error {
	client, err := r.getOrCreateSessionControlClientForWorkspace(ctx, agentID, workspacePath)
	if err != nil {
		return err
	}
	controller, ok := client.(middleware.ConversationSessionControl)
	if !ok || !controller.SessionCapabilities().Close {
		return fmt.Errorf("agent %s does not expose remote session close", agentID)
	}
	return controller.CloseRemoteSession(ctx, remoteSessionID)
}

func (r *Router) DeleteAgentSession(ctx context.Context, agentID string, remoteSessionID string) error {
	client, err := r.getOrCreateSessionControlClient(ctx, agentID)
	if err != nil {
		return err
	}
	controller, ok := client.(middleware.ConversationSessionControl)
	if !ok {
		return fmt.Errorf("agent %s does not expose remote session control", agentID)
	}
	return controller.DeleteRemoteSession(ctx, remoteSessionID)
}

func (r *Router) DeleteAgentSessionForWorkspace(ctx context.Context, agentID string, remoteSessionID string, workspacePath string) error {
	client, err := r.getOrCreateSessionControlClientForWorkspace(ctx, agentID, workspacePath)
	if err != nil {
		return err
	}
	controller, ok := client.(middleware.ConversationSessionControl)
	if !ok {
		return fmt.Errorf("agent %s does not expose remote session control", agentID)
	}
	return controller.DeleteRemoteSession(ctx, remoteSessionID)
}

func (r *Router) getOrCreateClient(ctx context.Context, agentID string, cwd string) (middleware.ConversationClient, error) {
	key := clientCacheKey(agentID, cwd)
	log := slog.With("component", "agent_router", "agent", agentID, "cwd", cwd)
	if client, ok := r.lookupReusableClient(key); ok {
		log.Debug("reusing cached conversation client", "event", "client_reused")
		return client, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if client, ok := r.lookupReusableClientLocked(key); ok {
		return client, nil
	}
	delete(r.clients, key)
	client, kind, err := r.createClient(ctx, agentID, cwd, log)
	if err != nil {
		return nil, err
	}
	log.Info("conversation client initialized", "event", "client_initialized", "protocol_kind", kind)
	r.clients[key] = client
	return client, nil
}

func (r *Router) getOrCreateSessionControlClient(ctx context.Context, agentID string) (middleware.ConversationClient, error) {
	if client, ok := r.lookupAnyReusableClientForAgent(agentID); ok {
		return client, nil
	}
	return r.getOrCreateClient(ctx, agentID, r.effectiveCwd(""))
}

func (r *Router) getOrCreateSessionControlClientForWorkspace(ctx context.Context, agentID string, workspacePath string) (middleware.ConversationClient, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return r.getOrCreateSessionControlClient(ctx, agentID)
	}
	return r.getOrCreateClient(ctx, agentID, r.effectiveCwd(workspacePath))
}

func (r *Router) lookupReusableClient(key string) (middleware.ConversationClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lookupReusableClientLocked(key)
}

func (r *Router) lookupReusableClientLocked(key string) (middleware.ConversationClient, bool) {
	client, ok := r.clients[key]
	if !ok || !isReusableClient(client) {
		return nil, false
	}
	return client, true
}

func (r *Router) lookupAnyReusableClientForAgent(agentID string) (middleware.ConversationClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for key, client := range r.clients {
		candidateAgentID, _ := splitClientCacheKey(key)
		if candidateAgentID == agentID && isReusableClient(client) {
			return client, true
		}
	}
	return nil, false
}

func (r *Router) effectiveCwd(workspacePath string) string {
	if strings.TrimSpace(workspacePath) != "" {
		return filepath.Clean(workspacePath)
	}
	if strings.TrimSpace(r.cwd) != "" {
		return filepath.Clean(r.cwd)
	}
	return "."
}

func clientCacheKey(agentID, cwd string) string {
	return agentID + "\x00" + filepath.Clean(cwd)
}

func splitClientCacheKey(key string) (string, string) {
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) != 2 {
		return key, ""
	}
	return parts[0], parts[1]
}

func isReusableClient(client middleware.ConversationClient) bool {
	if health, ok := client.(middleware.ConversationHealth); ok {
		return health.Alive()
	}
	return true
}
