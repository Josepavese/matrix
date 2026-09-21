// Package elicitation implements the neutral elicitation frontend as an
// in-memory pending registry. Ask registers a request and blocks until a
// channel frontend resolves it (HTTP API today), the context ends, or the
// per-request timeout expires. The Service is the single runtime source of
// truth for pending elicitations.
package elicitation

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// DefaultTimeout bounds how long an elicitation waits for a human answer
// before resolving as cancel. Agents get a bounded, predictable interaction
// window instead of an open-ended hang.
const DefaultTimeout = 120 * time.Second

// Pending is one unresolved elicitation as seen by channel frontends.
// ExpiresAt is what remains of the interaction window, so a UI can show it.
type Pending struct {
	Request   middleware.ElicitationRequest
	ExpiresAt time.Time
}

// pending is the internal registration record. Settlement is decided exactly
// once under Service.mu: whoever flips settled owns the outcome, so a racing
// answer or cancellation can never be reported as delivered and then dropped.
type pending struct {
	request   middleware.ElicitationRequest
	expiresAt time.Time
	seq       uint64
	settled   bool
	outcome   middleware.ElicitationOutcome
	done      chan struct{}
}

// EventKind classifies a registry lifecycle event.
type EventKind string

const (
	// EventOpened fires once a request is registered and answerable.
	EventOpened EventKind = "opened"
	// EventResolved fires once a request leaves the registry, whatever the
	// reason: an answer, a timeout, or a cancelled turn.
	EventResolved EventKind = "resolved"
)

// Event is a registry lifecycle notification. Channel frontends observe it to
// render and retract prompts; the registry stays the single source of truth.
type Event struct {
	Kind    EventKind
	Request middleware.ElicitationRequest
	// Outcome is set only for EventResolved.
	Outcome middleware.ElicitationOutcome
}

// Service implements middleware.ElicitationFrontend over an in-memory
// registry. It is safe for concurrent use.
type Service struct {
	mu      sync.Mutex
	next    map[string]*pending
	seq     uint64
	timeout time.Duration
	// subscribers receive lifecycle events through their own ordered queue, so
	// a slow or hanging channel frontend can never delay the answer that is on
	// its way to the agent.
	subscribers []*subscriber
}

// subscriberBuffer bounds how many lifecycle events may queue for one channel
// frontend before the oldest are dropped with a warning. It absorbs normal
// bursts without letting a stuck frontend grow memory without limit.
const subscriberBuffer = 64

// subscriber is one observer with an ordered queue and its own worker.
type subscriber struct {
	fn     func(Event)
	events chan Event
	stop   chan struct{}
	once   sync.Once
}

// Subscribe registers a lifecycle observer and returns its unsubscribe func.
// Events are delivered in order on a worker goroutine; a subscriber that blocks
// affects only its own queue.
func (s *Service) Subscribe(fn func(Event)) func() {
	if fn == nil {
		return func() {}
	}
	sub := &subscriber{
		fn:     fn,
		events: make(chan Event, subscriberBuffer),
		stop:   make(chan struct{}),
	}
	go sub.run()

	s.mu.Lock()
	s.subscribers = append(s.subscribers, sub)
	s.mu.Unlock()

	return func() {
		sub.once.Do(func() { close(sub.stop) })
		s.mu.Lock()
		defer s.mu.Unlock()
		for index, candidate := range s.subscribers {
			if candidate == sub {
				s.subscribers = append(s.subscribers[:index], s.subscribers[index+1:]...)
				return
			}
		}
	}
}

func (sub *subscriber) run() {
	for {
		select {
		case <-sub.stop:
			return
		case event := <-sub.events:
			sub.fn(event)
		}
	}
}

// publish hands the event to every subscriber without waiting for any of them.
func (s *Service) publish(event Event) {
	s.mu.Lock()
	subscribers := append([]*subscriber(nil), s.subscribers...)
	s.mu.Unlock()
	for _, sub := range subscribers {
		select {
		case sub.events <- event:
		default:
			slog.Warn("elicitation subscriber queue full; event dropped",
				"event", "elicitation_event_dropped", "kind", string(event.Kind),
				"elicitation_id", event.Request.ID)
		}
	}
}

var _ middleware.ElicitationFrontend = (*Service)(nil)

// NewService creates a frontend with the given response timeout. A zero or
// negative timeout falls back to DefaultTimeout.
func NewService(timeout time.Duration) *Service {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Service{next: map[string]*pending{}, timeout: timeout}
}

// Modes reports the modes the registry-backed frontend can serve: form and
// URL, because resolution is mode-agnostic — the channel decides how to
// render, the registry only correlates.
func (s *Service) Modes() []string {
	return []string{middleware.ElicitationModeForm, middleware.ElicitationModeURL}
}

// Ask registers the request and blocks until Respond resolves it, the
// context ends (cancel), or the timeout expires (cancel).
//
// Ask owns request identity: it assigns a unique ID derived from the request
// scope, because two agents can legitimately ask concurrent questions scoped
// to the same session and tool call. The assigned ID is published through
// Pending (for frontends listing work) and through Outcome.ID on return (for
// the caller's logs and correlation), so the scope key a caller passes is
// only a hint, never the identity.
func (s *Service) Ask(ctx context.Context, req middleware.ElicitationRequest) middleware.ElicitationOutcome {
	s.mu.Lock()
	if s.next == nil {
		s.next = map[string]*pending{}
	}
	s.seq++
	p := &pending{
		request:   req,
		expiresAt: time.Now().Add(s.timeout),
		seq:       s.seq,
		done:      make(chan struct{}),
	}
	p.request.ID = s.uniqueIDLocked(req.ID)
	s.next[p.request.ID] = p
	id := p.request.ID
	opened := p.request
	s.mu.Unlock()
	s.publish(Event{Kind: EventOpened, Request: opened})

	timer := time.NewTimer(s.timeout)
	defer timer.Stop()

	select {
	case <-p.done:
		return s.settledOutcome(id, p)
	case <-ctx.Done():
		return s.settle(id, p, middleware.ElicitationActionCancel)
	case <-timer.C:
		return s.settle(id, p, middleware.ElicitationActionCancel)
	}
}

// settledOutcome reads the outcome of an already settled request.
func (s *Service) settledOutcome(id string, p *pending) middleware.ElicitationOutcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	outcome := p.outcome
	outcome.ID = id
	return outcome
}

// uniqueIDLocked returns a free ID, suffixing the caller's scope key when it
// is already in use. Callers hold s.mu.
func (s *Service) uniqueIDLocked(base string) string {
	if base == "" {
		base = "elicitation"
	}
	if _, taken := s.next[base]; !taken {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s#%d", base, suffix)
		if _, taken := s.next[candidate]; !taken {
			return candidate
		}
	}
}

// Respond resolves a pending elicitation. It returns false when the ID is
// unknown or already settled — including when a timeout or a cancelled run won
// the race — so callers never report an answer that was not delivered.
func (s *Service) Respond(id string, outcome middleware.ElicitationOutcome) bool {
	settled, request, ok := s.settleLocked(id, outcome)
	if !ok {
		return false
	}
	s.publish(Event{Kind: EventResolved, Request: request, Outcome: outcome})
	close(settled)
	return true
}

// settle ends a request that reached its deadline or lost its turn, unless
// somebody already answered it. When the answer won, its outcome is returned
// instead of the cancellation: the agent must see the human's decision.
func (s *Service) settle(id string, p *pending, action string) middleware.ElicitationOutcome {
	cancel := middleware.ElicitationOutcome{ID: id, Action: action}
	settled, request, ok := s.settleLocked(id, cancel)
	if !ok {
		// Already settled elsewhere: return whichever outcome won.
		return s.settledOutcome(id, p)
	}
	s.publish(Event{Kind: EventResolved, Request: request, Outcome: cancel})
	close(settled)
	return cancel
}

// settleLocked atomically claims the single resolution slot for a request. It
// returns the channel to close once the outcome is stored, the request it
// belongs to, and whether this caller won. Callers hold s.mu and must publish
// only after it is released.
func (s *Service) settleLocked(id string, outcome middleware.ElicitationOutcome) (chan struct{}, middleware.ElicitationRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.next[id]
	if !ok || p.settled {
		return nil, middleware.ElicitationRequest{}, false
	}
	p.settled = true
	outcome.ID = id
	p.outcome = outcome
	delete(s.next, id)
	return p.done, p.request, true
}

// Pending returns a snapshot of the unresolved elicitations, oldest first.
func (s *Service) Pending() []Pending {
	s.mu.Lock()
	records := make([]*pending, 0, len(s.next))
	for _, p := range s.next {
		records = append(records, p)
	}
	s.mu.Unlock()
	sort.Slice(records, func(i, j int) bool { return records[i].seq < records[j].seq })
	out := make([]Pending, 0, len(records))
	for _, p := range records {
		out = append(out, Pending{Request: p.request, ExpiresAt: p.expiresAt})
	}
	return out
}
