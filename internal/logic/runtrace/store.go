package runtrace

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

const (
	runKeyPrefix       = "runtrace.run."
	eventKeyPrefix     = "runtrace.event."
	runEventsKeyPrefix = "runtrace.events."
	sinkKeyPrefix      = "runtrace.sink."
	maxRunEventRefs    = 1000
)

// Store persists Matrix run records and projects versioned trace views.
type Store struct {
	storage    middleware.Storage
	dispatcher func(Event)
	eventMu    sync.Mutex
}

func NewStore(storage middleware.Storage) *Store {
	return &Store{storage: storage}
}

func (s *Store) WithEventDispatcher(dispatcher func(Event)) *Store {
	if s != nil {
		s.dispatcher = dispatcher
	}
	return s
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
