// Package session manages channel-to-agent session routing for the Matrix runtime.
package session

import (
	"sync"

	"github.com/jose/matrix-v2/internal/middleware"
)

// EventBridge converts SessionObserver updates into SessionEvent channel sends.
// It allows the SessionManager to consume agent events in real-time.
type EventBridge struct {
	mu     sync.Mutex
	events chan middleware.SessionEvent
	closed bool
}

// NewEventBridge creates a bridge with a buffered event channel.
func NewEventBridge(buffer int) *EventBridge {
	return &EventBridge{
		events: make(chan middleware.SessionEvent, buffer),
	}
}

// OnUpdate implements SessionObserver — converts updates to events.
func (b *EventBridge) OnUpdate(notif middleware.SessionNotification) {
	if notif.Update.SessionUpdate == "agent_message_chunk" {
		b.send(middleware.SessionEvent{
			Type:    middleware.EventChunk,
			Content: notif.Update.Content.Text,
		})
	}
}

// Events returns the read-only channel for consuming events.
func (b *EventBridge) Events() <-chan middleware.SessionEvent {
	return b.events
}

func (b *EventBridge) send(ev middleware.SessionEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	select {
	case b.events <- ev:
	default:
		// Drop event if buffer is full — prevents blocking the observer
	}
}

// Close closes the event channel. Safe to call multiple times.
func (b *EventBridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	close(b.events)
}
