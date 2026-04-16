package runtrace

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Store) RegisterSink(sink Sink) (Sink, error) {
	if s == nil || s.storage == nil {
		return Sink{}, fmt.Errorf("run trace storage not available")
	}
	if strings.TrimSpace(sink.ID) == "" {
		sink.ID = "sink-" + uuid.NewString()
	}
	if err := validateSinkURL(sink.URL); err != nil {
		return Sink{}, err
	}
	if sink.CreatedAt.IsZero() {
		sink.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(sink)
	if err != nil {
		return Sink{}, fmt.Errorf("failed to encode event sink: %w", err)
	}
	if err := s.storage.Set(SinkKey(sink.ID), payload); err != nil {
		return Sink{}, fmt.Errorf("failed to store event sink: %w", err)
	}
	return sink, nil
}

func validateSinkURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("sink url must be absolute http or https")
	}
	return nil
}

func (s *Store) ListSinks() ([]Sink, error) {
	if s == nil || s.storage == nil {
		return nil, fmt.Errorf("run trace storage not available")
	}
	keys, err := s.storage.List(sinkKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list event sinks: %w", err)
	}
	sinks, err := s.loadSinks(keys)
	if err != nil {
		return nil, err
	}
	sort.Slice(sinks, func(i, j int) bool { return sinks[i].CreatedAt.Before(sinks[j].CreatedAt) })
	return sinks, nil
}

func (s *Store) loadSinks(keys []string) ([]Sink, error) {
	sinks := make([]Sink, 0, len(keys))
	for _, key := range keys {
		sink, found, err := s.loadSink(key)
		if err != nil {
			return nil, err
		}
		if found {
			sinks = append(sinks, sink)
		}
	}
	return sinks, nil
}

func (s *Store) loadSink(key string) (Sink, bool, error) {
	data, err := s.storage.Get(key)
	if err != nil {
		return Sink{}, false, fmt.Errorf("failed to read event sink %s: %w", key, err)
	}
	if len(data) == 0 {
		return Sink{}, false, nil
	}
	var sink Sink
	if err := json.Unmarshal(data, &sink); err != nil {
		return Sink{}, false, fmt.Errorf("failed to decode event sink %s: %w", key, err)
	}
	return sink, true, nil
}

func (s *Store) LoadSink(sinkID string) (Sink, bool, error) {
	return s.loadSink(SinkKey(sinkID))
}
