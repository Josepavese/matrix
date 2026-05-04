package agents

import (
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
	if r.clientTombstones == nil {
		r.clientTombstones = map[string]agentClientTombstone{}
	}
	now := time.Now()
	r.pruneClientTombstonesLocked(now)
	r.clientTombstones[key] = agentClientTombstone{
		reapedAt:         now,
		remoteSessionIDs: trackedRemoteSessionSet(client),
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
	delete(r.clientTombstones, key)
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

func tombstoneMatchesRemoteSession(tombstone agentClientTombstone, remoteSessionID string) bool {
	remoteSessionID = strings.TrimSpace(remoteSessionID)
	if remoteSessionID == "" {
		return true
	}
	if len(tombstone.remoteSessionIDs) == 0 {
		return false
	}
	_, ok := tombstone.remoteSessionIDs[remoteSessionID]
	return ok
}
