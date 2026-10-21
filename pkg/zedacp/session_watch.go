package zedacp

import (
	"context"
	"sync/atomic"
)

// WatchSessionUpdates observes live sessions for the connection lifetime,
// including compactions between prompt calls. It never replays private history.
// Callbacks run on the same ordered per-session worker as turn observers; callers
// synchronize state shared across sessions and release the returned registration.
func (c *Client) WatchSessionUpdates(observer SessionObserver) func() {
	if observer == nil {
		return func() {}
	}
	id := atomic.AddUint64(&c.nextObsID, 1)
	c.mu.Lock()
	if c.globalObservers == nil {
		c.globalObservers = make(map[uint64]SessionObserver)
	}
	c.globalObservers[id] = observer
	c.mu.Unlock()
	return func() { c.mu.Lock(); delete(c.globalObservers, id); c.mu.Unlock() }
}

// DrainSessionUpdates waits for already received updates, including callbacks in flight.
func (c *Client) DrainSessionUpdates(ctx context.Context, sessionID string) error {
	return c.drainSessionNotifications(ctx, sessionID)
}
