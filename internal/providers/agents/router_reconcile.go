package agents

import (
	"context"
	"fmt"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (r *Router) ReconcileAgentClients(_ context.Context, active []middleware.AgentClientRef) (middleware.AgentClientReconcileResult, error) {
	activeKeys := make(map[string][]middleware.AgentClientRef, len(active))
	for _, ref := range active {
		key := clientCacheKey(ref.AgentID, r.effectiveCwd(ref.WorkspacePath))
		activeKeys[key] = append(activeKeys[key], ref)
	}
	var toClose []clientToClose
	result := middleware.AgentClientReconcileResult{}
	r.mu.Lock()
	for key, client := range r.clients {
		agentID, cwd := splitClientCacheKey(key)
		ref := middleware.AgentClientRef{AgentID: agentID, WorkspacePath: cwd}
		if retained, ok := retainedAgentClientRef(activeKeys[key], client); ok {
			result.Retained = append(result.Retained, retained)
			continue
		}
		delete(r.clients, key)
		toClose = append(toClose, clientToClose{key: key, client: client})
		result.Reaped = append(result.Reaped, ref)
	}
	r.mu.Unlock()
	return result, closeReconciledClients(toClose)
}

func retainedAgentClientRef(active []middleware.AgentClientRef, client middleware.ConversationClient) (middleware.AgentClientRef, bool) {
	for _, ref := range active {
		if clientTracksRemoteSession(client, ref.RemoteSessionID) {
			return ref, true
		}
	}
	return middleware.AgentClientRef{}, false
}

type clientToClose struct {
	key    string
	client middleware.ConversationClient
}

func closeReconciledClients(clients []clientToClose) error {
	var closeErr error
	for _, item := range clients {
		if err := item.client.Close(); err != nil && closeErr == nil {
			closeErr = fmt.Errorf("close retained client %s: %w", item.key, err)
		}
	}
	return closeErr
}
