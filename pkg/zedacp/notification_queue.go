package zedacp

import (
	"encoding/json"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/Josepavese/matrix/pkg/zedacpstdio"
)

// A backlog may retain at most one transport frame's byte budget, including
// the callback in flight. Coalescing never makes the queue an unbounded buffer.
const notificationBacklogBytes = zedacpstdio.DefaultMaxFrameBytes

type sessionNotifications struct {
	mu      sync.Mutex
	queue   []observedNotification
	ready   chan struct{}
	bytes   int
	pending atomic.Int64
}

// Observer registrations belong to a turn. A queued update cannot migrate to
// the next turn's observer after cancellation removes its original owner.
type observedNotification struct {
	update    SessionNotification
	observers map[uint64]SessionObserver
	bytes     int
}

func (q *sessionNotifications) enqueue(next observedNotification) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if next.bytes > notificationBacklogBytes-q.bytes {
		return false
	}
	if n := len(q.queue); n > 0 && mergeNotificationText(&q.queue[n-1], next) {
		q.bytes += next.bytes
		return true
	}
	if len(q.queue) >= notificationQueueBuffer {
		return false
	}
	q.queue = append(q.queue, next)
	q.bytes += next.bytes
	q.pending.Add(1)
	q.signal()
	return true
}

func (q *sessionNotifications) signal() {
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

func (q *sessionNotifications) take() (observedNotification, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return observedNotification{}, false
	}
	next := q.queue[0]
	q.queue[0] = observedNotification{}
	q.queue = q.queue[1:]
	if len(q.queue) > 0 {
		q.signal()
	} else {
		q.queue = nil
	}
	return next, true
}

func (q *sessionNotifications) delivered(next observedNotification) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.bytes -= next.bytes
	q.pending.Add(-1)
}

// Merge only adjacent text chunks with identical envelopes and registrations.
// Message identity, phase, annotations, metadata and control updates are barriers.
// Exact text order and whitespace survive; no reasoning becomes answer content.
func mergeNotificationText(previous *observedNotification, next observedNotification) bool {
	if previous.update.SessionID != next.update.SessionID || !sameObserverIDs(previous.observers, next.observers) {
		return false
	}
	before, ok := textChunkEnvelope(previous.update.Update)
	after, otherOK := textChunkEnvelope(next.update.Update)
	if !ok || !otherOK || !reflect.DeepEqual(before, after) {
		return false
	}
	raw, ok := chunkContentEnvelope(previous.update.Update.RawContent)
	other, otherOK := chunkContentEnvelope(next.update.Update.RawContent)
	if !ok || !otherOK || !reflect.DeepEqual(raw, other) {
		return false
	}
	update := &previous.update.Update
	update.Content.Text += next.update.Update.Content.Text
	if len(update.Contents) == 1 {
		update.Contents = []Content{update.Content}
	}
	if raw != nil {
		raw["text"], _ = json.Marshal(update.Content.Text)
		update.RawContent, _ = json.Marshal(raw)
	}
	previous.bytes += next.bytes
	return true
}

func sameObserverIDs(left, right map[uint64]SessionObserver) bool {
	if len(left) != len(right) {
		return false
	}
	for id := range left {
		if _, ok := right[id]; !ok {
			return false
		}
	}
	return true
}

func textChunkEnvelope(update SessionUpdate) (SessionUpdate, bool) {
	if (update.SessionUpdate != "agent_message_chunk" && update.SessionUpdate != "agent_thought_chunk") ||
		update.Content.Type != "text" || len(update.Contents) > 1 || len(update.ToolContents) > 0 {
		return SessionUpdate{}, false
	}
	update.Content.Text = ""
	update.Contents, update.RawContent = nil, nil
	return update, true
}

func chunkContentEnvelope(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	var content map[string]json.RawMessage
	if json.Unmarshal(raw, &content) != nil || content == nil {
		return nil, false
	}
	delete(content, "text")
	return content, true
}
