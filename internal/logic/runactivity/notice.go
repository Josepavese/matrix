package runactivity

import (
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// SetLogicalSession preserves correlation when an activity observer decorates
// the run notifier before the session manager assigns the logical identity.
func (n *notifier) SetLogicalSession(sessionID, workspaceID string) {
	if forwarder, ok := n.inner.(interface{ SetLogicalSession(string, string) }); ok {
		forwarder.SetLogicalSession(sessionID, workspaceID)
	}
}

// WithNotice reports an explicitly configured silence interval once, without
// cancelling the turn. New observed activity rearms the notice. It cannot tell
// whether the provider is thinking, retrying, disconnected or out of credit.
func WithNotice(after time.Duration, inner middleware.ThoughtNotifier, notify func()) (middleware.ThoughtNotifier, func()) {
	if after <= 0 || notify == nil {
		return inner, func() {}
	}
	wrapped := &notifier{inner: inner, after: after, dueAt: time.Now().Add(after)}
	wrapped.timer = time.AfterFunc(after, func() {
		wrapped.mu.Lock()
		defer wrapped.mu.Unlock()
		if !wrapped.stopped && !time.Now().Before(wrapped.dueAt) {
			notify()
		}
	})
	return wrapped, func() {
		wrapped.mu.Lock()
		defer wrapped.mu.Unlock()
		wrapped.stopped = true
		wrapped.timer.Stop()
	}
}
