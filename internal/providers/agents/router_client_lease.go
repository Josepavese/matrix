package agents

import (
	"context"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (r *Router) acquireRouteClient(ctx context.Context, agentID, cwd string, launchArgs ...string) (middleware.ConversationClient, func(), error) {
	key := clientCacheKey(agentID, cwd, launchArgs...)
	r.mu.Lock()
	client, err := r.getOrCreateClientLocked(ctx, agentID, cwd, launchArgs...)
	if err != nil {
		r.mu.Unlock()
		return nil, nil, err
	}
	leaser, ok := client.(middleware.ConversationClientLeaser)
	if !ok {
		r.mu.Unlock()
		return client, func() {}, nil
	}
	release, err := leaser.AcquireConversationClientLease()
	r.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	return client, func() {
		closed, _ := release()
		if !closed {
			return
		}
		r.mu.Lock()
		r.removeDrainingClientLocked(key, client)
		r.rememberClientTombstoneLocked(key, client)
		r.mu.Unlock()
	}, nil
}

func (r *Router) lookupDrainingClient(agentID, cwd, remoteSessionID string) (middleware.ConversationClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for key, clients := range r.draining {
		if !clientCacheKeyMatchesBase(key, agentID, cwd) {
			continue
		}
		for _, client := range clients {
			if isReusableClient(client) && clientTracksRemoteSession(client, remoteSessionID) {
				return client, true
			}
		}
	}
	return nil, false
}

func (r *Router) removeDrainingClientLocked(key string, target middleware.ConversationClient) {
	clients := r.draining[key]
	for i, client := range clients {
		if !sameConversationClient(client, target) {
			continue
		}
		clients = append(clients[:i], clients[i+1:]...)
		break
	}
	if len(clients) == 0 {
		delete(r.draining, key)
		return
	}
	r.draining[key] = clients
}
