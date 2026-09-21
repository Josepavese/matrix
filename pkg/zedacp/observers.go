package zedacp

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
)

const responseObserverDrainIdle = 250 * time.Millisecond

// notificationQueueBuffer bounds the per-session notification backlog. A slow
// consumer loses updates rather than stalling the read loop.
const notificationQueueBuffer = 256

// notificationDrainPoll is how often a drain waits for the queue to empty.
const notificationDrainPoll = time.Millisecond

// sessionNotifications is the ordered delivery queue for one session. Updates
// are handed to observers on its own worker so a channel frontend that performs
// network I/O — the Telegram notifier does exactly that — cannot block JSON-RPC
// response correlation on the read loop.
type sessionNotifications struct {
	queue   chan SessionNotification
	pending atomic.Int64
}

// enqueueSessionUpdate hands an update to the session's worker without waiting.
func (c *Client) enqueueSessionUpdate(update SessionNotification) {
	queue := c.sessionNotificationQueue(update.SessionID)
	if queue == nil {
		return
	}
	queue.pending.Add(1)
	select {
	case queue.queue <- update:
	default:
		queue.pending.Add(-1)
		slog.Warn("dropping acp session update: notification queue full",
			"event", "acp_notification_dropped", "session", update.SessionID)
	}
}

func (c *Client) sessionNotificationQueue(sessionID string) *sessionNotifications {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	c.notifyMu.Lock()
	defer c.notifyMu.Unlock()
	if c.notifyQueues == nil {
		c.notifyQueues = make(map[string]*sessionNotifications)
	}
	if existing, ok := c.notifyQueues[sessionID]; ok {
		return existing
	}
	queue := &sessionNotifications{queue: make(chan SessionNotification, notificationQueueBuffer)}
	c.notifyQueues[sessionID] = queue
	// A client built without NewClient has no context: fall back to one that
	// never cancels rather than panicking on a nil dereference.
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go c.dispatchSessionUpdates(ctx, sessionID, queue)
	return queue
}

// dispatchSessionUpdates delivers one session's updates in order until the
// client closes.
func (c *Client) dispatchSessionUpdates(ctx context.Context, _ string, queue *sessionNotifications) {
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-queue.queue:
			c.deliverSessionUpdate(update)
			queue.pending.Add(-1)
		}
	}
}

// deliverSessionUpdate fans one update out to the session's observers. Each
// callback is isolated: a panicking frontend must neither kill the process nor
// steal the update from the other observers.
func (c *Client) deliverSessionUpdate(update SessionNotification) {
	for _, observer := range c.sessionObservers(update.SessionID) {
		if observer != nil {
			c.notifyObserverSafely(observer, update)
		}
	}
}

func (c *Client) notifyObserverSafely(observer SessionObserver, update SessionNotification) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("acp session observer panicked",
				"event", "acp_session_observer_panic", "session", update.SessionID, "panic", recovered)
		}
	}()
	observer.OnUpdate(update)
}

// drainSessionNotifications waits until the session's queue has been delivered,
// so callers that registered an observer for the duration of a call still see
// every update before the call returns.
func (c *Client) drainSessionNotifications(ctx context.Context, sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	deadline := time.Now().Add(responseObserverDrainIdle)
	for time.Now().Before(deadline) {
		c.notifyMu.Lock()
		queue, ok := c.notifyQueues[sessionID]
		c.notifyMu.Unlock()
		if !ok || queue.pending.Load() == 0 {
			return
		}
		if ctx != nil && ctx.Err() != nil {
			return
		}
		time.Sleep(notificationDrainPoll)
	}
	slog.Warn("timed out draining acp session updates", "event", "acp_notification_drain_timeout", "session", sessionID)
}

type observerIdleWaiter interface {
	WaitIdle(context.Context, time.Duration)
}

func (c *Client) registerObserver(sessionID string, observer SessionObserver) func() {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || observer == nil {
		return func() {}
	}
	id := atomic.AddUint64(&c.nextObsID, 1)
	c.mu.Lock()
	if c.observers == nil {
		c.observers = make(map[string]map[uint64]SessionObserver)
	}
	if c.observers[sessionID] == nil {
		c.observers[sessionID] = make(map[uint64]SessionObserver)
	}
	c.observers[sessionID][id] = observer
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		observers := c.observers[sessionID]
		delete(observers, id)
		if len(observers) == 0 {
			delete(c.observers, sessionID)
		}
	}
}

func (c *Client) sessionObservers(sessionID string) []SessionObserver {
	c.mu.RLock()
	defer c.mu.RUnlock()
	registered := c.observers[sessionID]
	if len(registered) == 0 {
		return nil
	}
	observers := make([]SessionObserver, 0, len(registered))
	for _, observer := range registered {
		if observer != nil {
			observers = append(observers, observer)
		}
	}
	return observers
}

// waitObserverIdle drains both queues a session update travels through: the
// client's delivery queue and whatever backlog the observer keeps itself.
//
// The order matters. Draining first delivers what the read loop already
// received; the observer then flushes its own backlog; draining again delivers
// anything that flush produced or that arrived meanwhile. Without the second
// drain an update emitted while the observer was catching up would be queued
// after the caller unregisters it and would be lost.
func (c *Client) waitObserverIdle(ctx context.Context, sessionID string, observer SessionObserver) {
	c.drainSessionNotifications(ctx, sessionID)
	if waiter, ok := observer.(observerIdleWaiter); ok && waiter != nil {
		waiter.WaitIdle(ctx, responseObserverDrainIdle)
	}
	c.drainSessionNotifications(ctx, sessionID)
}
