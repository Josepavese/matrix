package agents

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
)

func (r *Router) evictClientAfterTurnFailure(key string, client middleware.ConversationClient, remoteSessionID string, err error) bool {
	if !shouldEvictClientAfterTurnFailure(err) {
		return false
	}
	r.mu.Lock()
	current, ok := r.clients[key]
	if !ok || !sameConversationClient(current, client) {
		r.mu.Unlock()
		return false
	}
	r.rememberClientTombstoneWithRemoteLocked(key, current, remoteSessionID)
	delete(r.clients, key)
	r.mu.Unlock()
	_ = current.Close()
	agentID, cwd := splitClientCacheKey(key)
	slog.Info("evicted agent client after cancellable turn failure", "event", "turn_failure_client_evicted", "agent", agentID, "cwd", cwd, "remote_session_id", strings.TrimSpace(remoteSessionID), "error", err)
	return true
}

func shouldEvictClientAfterTurnFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if failure, ok := providerfailure.As(err); ok {
		reason := failure.Diagnostics["failure_reason"]
		return reason == "provider_client_context_cancelled" ||
			reason == "request_context_cancelled" ||
			strings.Contains(strings.ToLower(failure.Error()), "context deadline exceeded")
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "client context cancelled") ||
		strings.Contains(lower, "client context canceled") ||
		strings.Contains(lower, "context cancelled") ||
		strings.Contains(lower, "context canceled") ||
		strings.Contains(lower, "context deadline exceeded")
}
