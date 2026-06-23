package agents

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (r *Router) lookupReusableClientForAgentWorkspace(agentID, cwd string) (middleware.ConversationClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, key := range r.clientKeysForBaseLocked(agentID, cwd) {
		if client, ok := r.lookupReusableClientLocked(key); ok {
			return client, true
		}
	}
	return nil, false
}

func (r *Router) clientKeysForBaseLocked(agentID, cwd string) []string {
	keys := make([]string, 0, 1)
	for key := range r.clients {
		if clientCacheKeyMatchesBase(key, agentID, cwd) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func clientCacheKey(agentID, cwd string, launchArgs ...string) string {
	key := clientCacheBaseKey(agentID, cwd)
	if len(launchArgs) > 0 {
		key += "\x00" + strings.Join(launchArgs, "\x00")
	}
	return key
}

func clientCacheBaseKey(agentID, cwd string) string {
	return agentID + "\x00" + filepath.Clean(cwd)
}

func splitClientCacheKey(key string) (string, string) {
	agentID, cwd, _ := splitClientCacheKeyParts(key)
	return agentID, cwd
}

func splitClientCacheKeyParts(key string) (string, string, []string) {
	parts := strings.Split(key, "\x00")
	if len(parts) < 2 {
		return key, "", nil
	}
	return parts[0], parts[1], parts[2:]
}

func clientCacheKeyMatchesBase(key, agentID, cwd string) bool {
	candidateAgentID, candidateCwd := splitClientCacheKey(key)
	return candidateAgentID == agentID && filepath.Clean(candidateCwd) == filepath.Clean(cwd)
}
