package runtrace

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Store) AppendEvent(event Event) (Event, error) {
	if s == nil || s.storage == nil {
		return Event{}, fmt.Errorf("run trace storage not available")
	}
	event.RunID = strings.TrimSpace(event.RunID)
	if event.RunID == "" {
		return Event{}, fmt.Errorf("run id is required")
	}
	s.eventMu.Lock()
	defer s.eventMu.Unlock()

	resolved, err := s.resolveEventSequence(event)
	if err != nil {
		return Event{}, err
	}
	event, err = normalizeEvent(resolved)
	if err != nil {
		return Event{}, err
	}
	if err := s.storeEvent(event); err != nil {
		return Event{}, err
	}
	s.enqueueDispatch(event)
	return event, nil
}

// resolveEventSequence fills in a missing sequence number and keeps the per-run
// counter ahead of any explicitly numbered event, so a later automatic sequence
// can never collide with an earlier explicit one.
func (s *Store) resolveEventSequence(event Event) (Event, error) {
	if event.Sequence <= 0 {
		sequence, err := s.nextEventSequence(event.RunID)
		if err != nil {
			return Event{}, err
		}
		event.Sequence = sequence
		return event, nil
	}
	current, err := s.loadEventSequence(event.RunID)
	if err != nil {
		return Event{}, err
	}
	if event.Sequence > current {
		if err := s.saveEventSequence(event.RunID, event.Sequence); err != nil {
			return Event{}, err
		}
	}
	return event, nil
}

// storeEvent persists the payload and its index entry.
func (s *Store) storeEvent(event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to encode run event %s: %w", event.ID, err)
	}
	if err := s.storage.Set(EventKey(event.RunID, event.ID), payload); err != nil {
		return fmt.Errorf("failed to store run event %s: %w", event.ID, err)
	}
	return s.updateEventIndex(event.RunID, event.ID)
}

func normalizeEvent(event Event) (Event, error) {
	event.RunID = strings.TrimSpace(event.RunID)
	event.Kind = strings.TrimSpace(event.Kind)
	if event.RunID == "" {
		return Event{}, fmt.Errorf("run id is required")
	}
	if event.Kind == "" {
		return Event{}, fmt.Errorf("event kind is required")
	}
	if strings.TrimSpace(event.ID) == "" {
		event.ID = "evt-" + uuid.NewString()
	}
	if event.Actor == "" {
		event.Actor = "matrix"
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	return event, nil
}

func (s *Store) LoadEvents(runID string, limit int) ([]Event, error) {
	return s.LoadEventsAfter(runID, "", limit)
}

func (s *Store) LoadEventsAfter(runID, afterEventID string, limit int) ([]Event, error) {
	if s == nil || s.storage == nil {
		return nil, fmt.Errorf("run trace storage not available")
	}
	ids, err := s.loadEventIndex(runID)
	if err != nil {
		return nil, err
	}
	ids = idsAfter(ids, afterEventID)
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	events, err := s.loadEventsByID(runID, ids)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Sequence > 0 && events[j].Sequence > 0 {
			return events[i].Sequence < events[j].Sequence
		}
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return events, nil
}

// idsAfter returns the ids following a cursor. When the cursor has been evicted
// from the retained window it returns the whole window, which is at-least-once
// delivery: a client may see an update twice, but never misses one that is still
// retained.
func idsAfter(ids []string, afterEventID string) []string {
	if strings.TrimSpace(afterEventID) == "" {
		return ids
	}
	for i, id := range ids {
		if id == afterEventID && i+1 < len(ids) {
			return ids[i+1:]
		}
		if id == afterEventID {
			return nil
		}
	}
	return ids
}

func (s *Store) loadEventsByID(runID string, ids []string) ([]Event, error) {
	events := make([]Event, 0, len(ids))
	for _, eventID := range ids {
		event, found, err := s.loadEvent(runID, eventID)
		if err != nil {
			return nil, err
		}
		if found {
			events = append(events, event)
		}
	}
	return events, nil
}

func (s *Store) loadEvent(runID, eventID string) (Event, bool, error) {
	data, err := s.storage.Get(EventKey(runID, eventID))
	if err != nil {
		return Event{}, false, fmt.Errorf("failed to read run event %s: %w", eventID, err)
	}
	if len(data) == 0 {
		return Event{}, false, nil
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return Event{}, false, fmt.Errorf("failed to decode run event %s: %w", eventID, err)
	}
	return event, true, nil
}

func (s *Store) LoadEvent(runID, eventID string) (Event, bool, error) {
	return s.loadEvent(runID, eventID)
}

func (s *Store) updateEventIndex(runID, eventID string) error {
	ids, err := s.loadEventIndex(runID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id == eventID {
			return nil
		}
	}
	ids = append(ids, eventID)
	if len(ids) > maxRunEventRefs {
		// The index keeps the newest window. Drop the payloads that fall out of
		// it as well: leaving them behind grew storage without limit for the
		// lifetime of a run while no reader could ever reach them again. The
		// newest events — including the terminal ones — are the ones retained.
		for _, evicted := range ids[:len(ids)-maxRunEventRefs] {
			if err := s.storage.Delete(EventKey(runID, evicted)); err != nil {
				slog.Warn("failed to delete evicted run event", "event", "run_event_evict_failed", "run_id", runID, "event_id", evicted, "error", err)
			}
		}
		ids = ids[len(ids)-maxRunEventRefs:]
	}
	payload, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("failed to encode run event index: %w", err)
	}
	if err := s.storage.Set(RunEventsKey(runID), payload); err != nil {
		return fmt.Errorf("failed to store run event index: %w", err)
	}
	return nil
}

func (s *Store) loadEventIndex(runID string) ([]string, error) {
	if s == nil || s.storage == nil {
		return nil, fmt.Errorf("run trace storage not available")
	}
	data, err := s.storage.Get(RunEventsKey(runID))
	if err != nil {
		return nil, fmt.Errorf("failed to read run event index: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("failed to decode run event index: %w", err)
	}
	return ids, nil
}
