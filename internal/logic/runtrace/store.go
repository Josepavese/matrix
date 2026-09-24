package runtrace

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

const (
	runKeyPrefix         = "runtrace.run."
	eventKeyPrefix       = "runtrace.event."
	runEventsKeyPrefix   = "runtrace.events."
	runEventSeqKeyPrefix = "runtrace.eventseq."
	sinkKeyPrefix        = "runtrace.sink."
	maxRunEventRefs      = 1000
)

// runEventDispatchBuffer bounds how many appended events may wait for the
// dispatcher before new ones are dropped with a warning.
const runEventDispatchBuffer = 1024

// Store persists Matrix run records and projects versioned trace views.
type Store struct {
	storage                         middleware.Storage
	eventMu                         sync.Mutex
	idempotencyMu                   sync.Mutex
	notificationMu                  sync.Mutex
	notificationSequenceInitialized bool

	// The dispatcher is fed by one ordered worker rather than a goroutine per
	// event, so sinks see events in sequence order and a panicking dispatcher
	// cannot take the daemon down with it.
	dispatchMu    sync.Mutex
	dispatcher    func(Event)
	dispatchQueue chan Event
	dispatchOnce  sync.Once
	droppedEvents int

	// transitionLocks serialise the terminal transition of a run. Striped
	// rather than per-run so the map cannot grow with the number of runs.
	transitionLocks [transitionLockStripes]sync.Mutex
}

// transitionLockStripes is the number of stripes guarding run transitions.
const transitionLockStripes = 64

// transitionLock returns the lock guarding one run's terminal transition.
func (s *Store) transitionLock(runID string) *sync.Mutex {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(runID))
	return &s.transitionLocks[hash.Sum32()%transitionLockStripes]
}

// emitTerminalTrace runs the trace writes that follow a durably stored terminal
// transition. A trace failure is logged, never reported as a failed run: the
// run did reach its terminal state, and a caller that saw an error could
// re-execute work that already happened.
func (s *Store) emitTerminalTrace(run Run, steps ...func() error) {
	for _, step := range steps {
		if step == nil {
			continue
		}
		if err := step(); err != nil {
			slog.Error("failed to append terminal run trace",
				"event", "run_terminal_trace_failed", "run_id", run.ID, "status", run.Status, "error", err)
		}
	}
}

// runEventDropLogEvery rate-limits the drop warning: a stalled sink would
// otherwise emit one log line per produced event and drown the log itself.
const runEventDropLogEvery = 256

func NewStore(storage middleware.Storage) *Store {
	return &Store{storage: storage}
}

func (s *Store) WithEventDispatcher(dispatcher func(Event)) *Store {
	if s != nil {
		s.dispatchMu.Lock()
		s.dispatcher = dispatcher
		s.dispatchMu.Unlock()
	}
	return s
}

func (s *Store) currentDispatcher() func(Event) {
	s.dispatchMu.Lock()
	defer s.dispatchMu.Unlock()
	return s.dispatcher
}

// enqueueDispatch hands an event to the single dispatch worker. It never blocks
// the caller: a stalled sink must not stall the run that produced the event.
func (s *Store) enqueueDispatch(event Event) {
	if s.currentDispatcher() == nil {
		return
	}
	s.dispatchOnce.Do(func() {
		s.dispatchQueue = make(chan Event, runEventDispatchBuffer)
		go s.dispatchLoop(s.dispatchQueue)
	})
	select {
	case s.dispatchQueue <- event:
	default:
		s.dispatchMu.Lock()
		s.droppedEvents++
		dropped := s.droppedEvents
		s.dispatchMu.Unlock()
		if dropped == 1 || dropped%runEventDropLogEvery == 0 {
			slog.Warn("dropping run events: dispatch queue full",
				"event", "run_event_dispatch_dropped", "run_id", event.RunID,
				"dropped_total", dropped)
		}
	}
}

func (s *Store) dispatchLoop(queue chan Event) {
	for event := range queue {
		s.dispatchOne(event)
	}
}

// dispatchOne keeps a dispatcher panic inside the worker: losing one event is
// survivable, losing the process is not.
func (s *Store) dispatchOne(event Event) {
	dispatcher := s.currentDispatcher()
	if dispatcher == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("run event dispatcher panicked",
				"event", "run_event_dispatcher_panic", "run_id", event.RunID, "panic", recovered)
		}
	}()
	dispatcher(event)
}

// RunEventSequenceKey stores the per-run monotonic event counter.
func RunEventSequenceKey(runID string) string {
	return runEventSeqKeyPrefix + runID
}

// nextEventSequence allocates the next sequence number for a run. It must not
// be derived from the event index length: that index is capped, so counting it
// would repeat sequences once a run crosses the cap.
func (s *Store) nextEventSequence(runID string) (int, error) {
	current, err := s.loadEventSequence(runID)
	if err != nil {
		return 0, err
	}
	next := current + 1
	if err := s.saveEventSequence(runID, next); err != nil {
		return 0, err
	}
	return next, nil
}

func (s *Store) loadEventSequence(runID string) (int, error) {
	data, err := s.storage.Get(RunEventSequenceKey(runID))
	if err != nil {
		return 0, fmt.Errorf("failed to read run event counter for %s: %w", runID, err)
	}
	if len(data) == 0 {
		// Vaults written before the counter existed: take the highest sequence
		// already stored so numbering continues instead of restarting.
		return s.eventSequenceFloor(runID)
	}
	var value int
	if err := json.Unmarshal(data, &value); err != nil {
		return 0, fmt.Errorf("failed to decode run event counter for %s: %w", runID, err)
	}
	return value, nil
}

func (s *Store) saveEventSequence(runID string, value int) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to encode run event counter for %s: %w", runID, err)
	}
	if err := s.storage.Set(RunEventSequenceKey(runID), payload); err != nil {
		return fmt.Errorf("failed to store run event counter for %s: %w", runID, err)
	}
	return nil
}

// eventSequenceFloor reconstructs the counter for a run whose counter is absent
// and raises it above any explicitly supplied sequence.
func (s *Store) eventSequenceFloor(runID string) (int, error) {
	ids, err := s.loadEventIndex(runID)
	if err != nil {
		return 0, err
	}
	highest := 0
	for _, id := range ids {
		event, found, err := s.loadEvent(runID, id)
		if err != nil || !found {
			continue
		}
		if event.Sequence > highest {
			highest = event.Sequence
		}
	}
	return highest, nil
}

func RunKey(runID string) string {
	return runKeyPrefix + runID
}

func EventKey(runID, eventID string) string {
	return eventKeyPrefix + runID + "." + eventID
}

func RunEventsKey(runID string) string {
	return runEventsKeyPrefix + runID
}

func SinkKey(sinkID string) string {
	return sinkKeyPrefix + sinkID
}

func (s *Store) SaveRun(run Run) error {
	if s == nil || s.storage == nil {
		return fmt.Errorf("run trace storage not available")
	}
	if strings.TrimSpace(run.ID) == "" {
		return fmt.Errorf("run id is required")
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("failed to encode run %s: %w", run.ID, err)
	}
	if err := s.storage.Set(RunKey(run.ID), payload); err != nil {
		return fmt.Errorf("failed to store run %s: %w", run.ID, err)
	}
	return nil
}

func (s *Store) LoadRun(runID string) (Run, bool, error) {
	if s == nil || s.storage == nil {
		return Run{}, false, fmt.Errorf("run trace storage not available")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return Run{}, false, fmt.Errorf("run id is required")
	}
	data, err := s.storage.Get(RunKey(runID))
	if err != nil {
		return Run{}, false, fmt.Errorf("failed to read run %s: %w", runID, err)
	}
	if len(data) == 0 {
		return Run{}, false, nil
	}
	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		return Run{}, false, fmt.Errorf("failed to decode run %s: %w", runID, err)
	}
	return run, true, nil
}
