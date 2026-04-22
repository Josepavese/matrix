package sessionqueue

import (
	"log/slog"
	"sync"

	"github.com/jose/matrix-v2/internal/middleware"
)

type RouteResult struct {
	LogicalSessionID string
	Content          string
	AgentSessionID   string
	Metadata         middleware.ConversationMetadata
	Err              error
}

type OrderedMerge struct {
	mu         sync.Mutex
	nextSeq    int
	lastMerged int
	pending    map[int]RouteResult
	onFlush    func(seq int, result RouteResult)
}

func New(onFlush func(seq int, result RouteResult)) *OrderedMerge {
	return &OrderedMerge{
		lastMerged: -1,
		pending:    make(map[int]RouteResult),
		onFlush:    onFlush,
	}
}

func (m *OrderedMerge) NextSeq() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	seq := m.nextSeq
	m.nextSeq++
	return seq
}

func (m *OrderedMerge) Submit(seq int, result RouteResult) {
	m.mu.Lock()
	m.pending[seq] = result
	flushed := m.flushLocked()
	m.mu.Unlock()
	if flushed > 0 {
		slog.Debug("ordered merge flushed", "event", "merge_flush", "flushed", flushed, "last_merged", m.lastMerged, "pending", len(m.pending))
	}
}

func (m *OrderedMerge) flushLocked() int {
	flushed := 0
	for {
		next := m.lastMerged + 1
		result, ok := m.pending[next]
		if !ok {
			return flushed
		}
		delete(m.pending, next)
		m.lastMerged = next
		if m.onFlush != nil {
			m.onFlush(next, result)
		}
		flushed++
	}
}
