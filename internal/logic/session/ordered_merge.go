package session

import (
	"log/slog"
	"sync"

	"github.com/jose/matrix-v2/internal/middleware"
)

// RouteResult holds the outcome of a single routing call.
type RouteResult struct {
	Content        string
	AgentSessionID string
	Metadata       middleware.ConversationMetadata
	Err            error
}

// OrderedMerge reorders concurrent results by sequence number.
// Inspired by TCP reassembly: results are stored in a pending map,
// then flushed consecutively starting from lastMerged+1.
type OrderedMerge struct {
	mu         sync.Mutex
	nextSeq    int
	lastMerged int
	pending    map[int]*mergeEntry
	onFlush    func(seq int, result RouteResult)
}

type mergeEntry struct {
	result RouteResult
}

// NewOrderedMerge creates a new merge coordinator.
// onFlush is called for each result in order when it becomes ready.
func NewOrderedMerge(onFlush func(seq int, result RouteResult)) *OrderedMerge {
	return &OrderedMerge{
		nextSeq:    0,
		lastMerged: -1,
		pending:    make(map[int]*mergeEntry),
		onFlush:    onFlush,
	}
}

// NextSeq assigns the next sequence number under lock.
func (m *OrderedMerge) NextSeq() int {
	m.mu.Lock()
	seq := m.nextSeq
	m.nextSeq++
	m.mu.Unlock()
	return seq
}

// Submit stores a result and flushes consecutive entries.
// This is the gap-filling ordered merge from brain's mergeResponse pattern.
func (m *OrderedMerge) Submit(seq int, result RouteResult) {
	m.mu.Lock()
	m.pending[seq] = &mergeEntry{result: result}

	flushed := 0
	for {
		next := m.lastMerged + 1
		p, ok := m.pending[next]
		if !ok {
			break
		}
		delete(m.pending, next)
		m.lastMerged = next
		if m.onFlush != nil {
			m.onFlush(next, p.result)
		}
		flushed++
	}
	m.mu.Unlock()

	if flushed > 0 {
		slog.Debug("ordered merge flushed", "event", "merge_flush", "flushed", flushed, "last_merged", m.lastMerged, "pending", len(m.pending))
	}
}
