package runtrace

import (
	"encoding/json"
	"fmt"
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
	if event.Sequence <= 0 {
		ids, err := s.loadEventIndex(event.RunID)
		if err != nil {
			return Event{}, err
		}
		event.Sequence = len(ids) + 1
	}
	event, err := normalizeEvent(event)
	if err != nil {
		return Event{}, err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return Event{}, fmt.Errorf("failed to encode run event %s: %w", event.ID, err)
	}
	if err := s.storage.Set(EventKey(event.RunID, event.ID), payload); err != nil {
		return Event{}, fmt.Errorf("failed to store run event %s: %w", event.ID, err)
	}
	err = s.updateEventIndex(event.RunID, event.ID)
	if err != nil {
		return Event{}, err
	}
	if s.dispatcher != nil {
		go s.dispatcher(event)
	}
	return event, nil
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
