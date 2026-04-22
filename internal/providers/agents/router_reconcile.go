package agents

import (
	"context"
	"fmt"

	"github.com/jose/matrix-v2/internal/middleware"
)

func (r *Router) ReconcileAgentClients(_ context.Context, active []middleware.AgentClientRef) (middleware.AgentClientReconcileResult, error) {
	activeKeys := make(map[string]struct{}, len(active))
	for _, ref := range active {
		activeKeys[clientCacheKey(ref.AgentID, r.effectiveCwd(ref.WorkspacePath))] = struct{}{}
	}
	var toClose []clientToClose
	result := middleware.AgentClientReconcileResult{}
	r.mu.Lock()
	for key, client := range r.clients {
		agentID, cwd := splitClientCacheKey(key)
		ref := middleware.AgentClientRef{AgentID: agentID, WorkspacePath: cwd}
		if _, ok := activeKeys[key]; ok {
			result.Retained = append(result.Retained, ref)
			continue
		}
		delete(r.clients, key)
		toClose = append(toClose, clientToClose{key: key, client: client})
		result.Reaped = append(result.Reaped, ref)
	}
	r.mu.Unlock()
	return result, closeReconciledClients(toClose)
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
