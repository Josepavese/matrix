package agents

import (
	"reflect"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

type agentClientTombstone struct {
	reapedAt         time.Time
	remoteSessionIDs map[string]struct{}
}

type trackedRemoteSessionClient interface {
	TrackedRemoteSessionIDs() []string
}

func (r *Router) rememberClientTombstoneLocked(key string, client middleware.ConversationClient) {
	r.rememberClientTombstoneWithRemoteLocked(key, client, "")
}

func (r *Router) rememberClientTombstoneWithRemoteLocked(key string, client middleware.ConversationClient, remoteSessionID string) {
	if r.clientTombstones == nil {
		r.clientTombstones = map[string]agentClientTombstone{}
	}
	now := time.Now()
	r.pruneClientTombstonesLocked(now)
	remoteSessionIDs := trackedRemoteSessionSet(client)
	remoteSessionID = strings.TrimSpace(remoteSessionID)
	if remoteSessionID != "" {
		if remoteSessionIDs == nil {
			remoteSessionIDs = map[string]struct{}{}
		}
		remoteSessionIDs[remoteSessionID] = struct{}{}
	}
	if existing, ok := r.clientTombstones[key]; ok {
		remoteSessionIDs = mergeRemoteSessionSets(existing.remoteSessionIDs, remoteSessionIDs)
	}
	r.clientTombstones[key] = agentClientTombstone{
		reapedAt:         now,
		remoteSessionIDs: remoteSessionIDs,
	}
}

func (r *Router) consumeClientTombstoneLocked(key string, remoteSessionID string) bool {
	if r.clientTombstones == nil {
		return false
	}
	r.pruneClientTombstonesLocked(time.Now())
	tombstone, ok := r.clientTombstones[key]
	if !ok || !tombstoneMatchesRemoteSession(tombstone, remoteSessionID) {
		return false
	}
	if remoteSessionID == "" || len(tombstone.remoteSessionIDs) == 0 {
		delete(r.clientTombstones, key)
		return true
	}
	delete(tombstone.remoteSessionIDs, remoteSessionID)
	if len(tombstone.remoteSessionIDs) == 0 {
		delete(r.clientTombstones, key)
	} else {
		r.clientTombstones[key] = tombstone
	}
	return true
}

func (r *Router) pruneClientTombstonesLocked(now time.Time) {
	for key, tombstone := range r.clientTombstones {
		if now.Sub(tombstone.reapedAt) > clientReapTombstoneTTL {
			delete(r.clientTombstones, key)
		}
	}
}

func clientTracksRemoteSession(client middleware.ConversationClient, remoteSessionID string) bool {
	remoteSessionID = strings.TrimSpace(remoteSessionID)
	if remoteSessionID == "" {
		return true
	}
	if tracked, ok := client.(trackedRemoteSessionClient); ok {
		return remoteSessionTracked(tracked.TrackedRemoteSessionIDs(), remoteSessionID)
	}
	return true
}

func remoteSessionTracked(ids []string, remoteSessionID string) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == remoteSessionID {
			return true
		}
	}
	return false
}

func trackedRemoteSessionSet(client middleware.ConversationClient) map[string]struct{} {
	tracked, ok := client.(trackedRemoteSessionClient)
	if !ok {
		return nil
	}
	out := map[string]struct{}{}
	for _, id := range tracked.TrackedRemoteSessionIDs() {
		id = strings.TrimSpace(id)
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}

func mergeRemoteSessionSets(a, b map[string]struct{}) map[string]struct{} {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for id := range a {
		out[id] = struct{}{}
	}
	for id := range b {
		out[id] = struct{}{}
	}
	return out
}

func tombstoneMatchesRemoteSession(tombstone agentClientTombstone, remoteSessionID string) bool {
	remoteSessionID = strings.TrimSpace(remoteSessionID)
	if remoteSessionID == "" {
		return len(tombstone.remoteSessionIDs) == 0
	}
	if len(tombstone.remoteSessionIDs) == 0 {
		return false
	}
	_, ok := tombstone.remoteSessionIDs[remoteSessionID]
	return ok
}

func (r *Router) evictDeadClientForLifecycle(key string) bool {
	r.mu.Lock()
	client, ok := r.clients[key]
	if ok && isReusableClient(client) {
		r.mu.Unlock()
		return false
	}
	if ok {
		r.rememberClientTombstoneLocked(key, client)
		delete(r.clients, key)
	}
	r.mu.Unlock()
	if ok {
		_ = client.Close()
	}
	return ok
}

func sameConversationClient(a, b middleware.ConversationClient) bool {
	if a == nil || b == nil {
		return false
	}
	ta := reflect.TypeOf(a)
	if ta != reflect.TypeOf(b) || !ta.Comparable() {
		return false
	}
	return a == b
}
