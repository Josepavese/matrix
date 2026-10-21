package zedacp

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
)

const responseObserverDrainIdle = 250 * time.Millisecond

// notificationQueueBuffer bounds the per-session notification backlog. A slow
// consumer fails the connection rather than silently losing turn evidence or
// stalling JSON-RPC response correlation on the read loop.
const notificationQueueBuffer = 256

// ErrNotificationOverflow means the peer's stream could not be delivered in
// full. A caller must not accept the partial turn as a successful response.
var ErrNotificationOverflow = errors.New("acp notification delivery overflow")

// notificationDrainPoll is how often a drain waits for the queue to empty.
const notificationDrainPoll = time.Millisecond

// sessionNotifications is the ordered delivery queue for one session. Updates
// are handed to observers on its own worker so a channel frontend that performs
// network I/O — the Telegram notifier does exactly that — cannot block JSON-RPC
// response correlation on the read loop.
type sessionNotifications struct {
	queue   chan observedNotification
	pending atomic.Int64
}

// Observer registrations belong to a turn. A queued update must never migrate
// to the next turn's observer when cancellation removes its original owner.
type observedNotification struct {
	update    SessionNotification
	observers map[uint64]SessionObserver
}

// enqueueSessionUpdate hands an update to the session's worker without waiting.
func (c *Client) enqueueSessionUpdate(update SessionNotification) {
	targets := c.sessionObserverSnapshot(update.SessionID, nil)
	if len(targets) == 0 {
		return
	}
	queue := c.sessionNotificationQueue(update.SessionID)
	queue.pending.Add(1)
	select {
	case queue.queue <- observedNotification{update: update, observers: targets}:
	default:
		queue.pending.Add(-1)
		c.fail(ErrNotificationOverflow)
	}
}

func (c *Client) sessionNotificationQueue(sessionID string) *sessionNotifications {
	sessionID = strings.TrimSpace(sessionID)
	c.notifyMu.Lock()
	defer c.notifyMu.Unlock()
	if c.notifyQueues == nil {
		c.notifyQueues = make(map[string]*sessionNotifications)
	}
	if existing, ok := c.notifyQueues[sessionID]; ok {
		return existing
	}
	queue := &sessionNotifications{queue: make(chan observedNotification, notificationQueueBuffer)}
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
func (c *Client) deliverSessionUpdate(notification observedNotification) {
	update := notification.update
	for _, observer := range c.sessionObserverSnapshot(update.SessionID, notification.observers) {
		c.notifyObserverSafely(observer, update)
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
func (c *Client) drainSessionNotifications(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	ticker := time.NewTicker(notificationDrainPoll)
	defer ticker.Stop()
	for {
		if err := c.deliveryError(ctx); err != nil {
			return err
		}
		c.notifyMu.Lock()
		queue, ok := c.notifyQueues[sessionID]
		c.notifyMu.Unlock()
		if !ok || queue.pending.Load() == 0 {
			return nil
		}
		// Completion is delivery, not a fixed grace period. The caller's
		// context and the client lifetime bound a stalled observer.
		<-ticker.C
	}
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

func (c *Client) sessionObserverSnapshot(sessionID string, targets map[uint64]SessionObserver) map[uint64]SessionObserver {
	c.mu.RLock()
	defer c.mu.RUnlock()
	registered := c.observers[sessionID]
	observers := make(map[uint64]SessionObserver, len(registered))
	for id, observer := range registered {
		if observer != nil && (targets == nil || targets[id] != nil) {
			observers[id] = observer
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
func (c *Client) waitObserverIdle(ctx context.Context, sessionID string, observer SessionObserver) error {
	if err := c.drainSessionNotifications(ctx, sessionID); err != nil {
		return err
	}
	if waiter, ok := observer.(observerIdleWaiter); ok && waiter != nil {
		waiter.WaitIdle(ctx, responseObserverDrainIdle)
	}
	return c.drainSessionNotifications(ctx, sessionID)
}
