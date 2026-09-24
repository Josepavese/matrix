package runtrace

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const notificationPrefix = "runtrace.notification."
const notificationSequenceKey = "runtrace.notification_sequence"

// Notification is the low-content wakeup envelope for local supervisors.
// Prompt, transcript, tool output and reasoning never enter this record.
type Notification struct {
	Sequence      uint64    `json:"sequence"`
	Kind          string    `json:"kind"`
	RunID         string    `json:"run_id,omitempty"`
	AgentID       string    `json:"agent_id,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`
	ElicitationID string    `json:"elicitation_id,omitempty"`
	FailureCode   string    `json:"failure_code,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

func (s *Store) AppendNotification(notification Notification) (Notification, error) {
	if s == nil || s.storage == nil {
		return Notification{}, fmt.Errorf("run trace storage not available")
	}
	s.notificationMu.Lock()
	defer s.notificationMu.Unlock()
	sequence, err := s.nextNotificationSequence()
	if err != nil {
		return Notification{}, err
	}
	notification.Sequence = sequence
	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now().UTC()
	}
	encoded, err := json.Marshal(notification)
	if err != nil {
		return Notification{}, err
	}
	if err := s.storage.Set(fmt.Sprintf("%s%020d", notificationPrefix, sequence), encoded); err != nil {
		return Notification{}, err
	}
	if err := s.storage.Set(notificationSequenceKey, []byte(strconv.FormatUint(sequence, 10))); err != nil {
		return Notification{}, err
	}
	s.notificationSequenceInitialized = true
	return notification, nil
}

func (s *Store) nextNotificationSequence() (uint64, error) {
	data, err := s.storage.Get(notificationSequenceKey)
	if err != nil {
		return 0, err
	}
	sequence := uint64(0)
	if len(data) != 0 {
		sequence, err = strconv.ParseUint(string(data), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid notification sequence: %w", err)
		}
	}
	if s.notificationSequenceInitialized {
		return sequence + 1, nil
	}
	// A crash may occur after storing a numbered notification and before
	// updating the counter. Rebuild the floor once on process startup.
	keys, err := s.storage.List(notificationPrefix)
	if err != nil {
		return 0, err
	}
	for _, item := range keys {
		stored, parseErr := strconv.ParseUint(strings.TrimPrefix(item, notificationPrefix), 10, 64)
		if parseErr == nil && stored > sequence {
			sequence = stored
		}
	}
	return sequence + 1, nil
}

// ReconcileTerminalNotifications restores wakeups if the daemon stopped
// between persisting a terminal run and persisting its notification. It runs
// before ingress on startup and never replays an agent prompt.
func (s *Store) ReconcileTerminalNotifications() (int, error) {
	keys, err := s.storage.List(runKeyPrefix)
	if err != nil {
		return 0, err
	}
	seen, err := s.existingTerminalWakeups()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, key := range keys {
		run, found, err := s.LoadRun(strings.TrimPrefix(key, runKeyPrefix))
		if err != nil {
			return count, err
		}
		if !found || !isTerminalStatus(run.Status) || seen[run.ID] {
			continue
		}
		kind := "run." + run.Status
		if run.Status == StatusUnknown {
			kind = "run.outcome_unknown"
		}
		if _, err := s.AppendNotification(Notification{Kind: kind, RunID: run.ID, FailureCode: run.StopReason, Timestamp: run.CompletedAt}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Store) existingTerminalWakeups() (map[string]bool, error) {
	notificationKeys, err := s.storage.List(notificationPrefix)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(notificationKeys))
	for _, key := range notificationKeys {
		data, err := s.storage.Get(key)
		if err != nil {
			return nil, err
		}
		var item Notification
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if isWakeupEvent(item.Kind) {
			seen[item.RunID] = true
		}
	}
	return seen, nil
}

func (s *Store) LoadNotificationsAfter(after uint64, limit int, runIDs map[string]struct{}) ([]Notification, uint64, error) {
	if s == nil || s.storage == nil {
		return nil, after, fmt.Errorf("run trace storage not available")
	}
	keys, err := s.storage.List(notificationPrefix)
	if err != nil {
		return nil, after, err
	}
	sort.Strings(keys)
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	out := make([]Notification, 0, limit)
	cursor := after
	for _, key := range keys {
		seq, ok := notificationSequenceAfter(key, after)
		if !ok {
			continue
		}
		notification, err := s.loadNotification(key)
		if err != nil {
			return nil, after, err
		}
		cursor = seq
		if !notificationMatchesRun(notification, runIDs) {
			continue
		}
		out = append(out, notification)
		if len(out) == limit {
			break
		}
	}
	return out, cursor, nil
}

func notificationSequenceAfter(key string, after uint64) (uint64, bool) {
	sequence, err := strconv.ParseUint(strings.TrimPrefix(key, notificationPrefix), 10, 64)
	return sequence, err == nil && sequence > after
}

func notificationMatchesRun(notification Notification, runIDs map[string]struct{}) bool {
	if len(runIDs) == 0 {
		return true
	}
	_, ok := runIDs[notification.RunID]
	return ok
}

func (s *Store) loadNotification(key string) (Notification, error) {
	data, err := s.storage.Get(key)
	if err != nil {
		return Notification{}, err
	}
	var notification Notification
	if err := json.Unmarshal(data, &notification); err != nil {
		return Notification{}, err
	}
	return notification, nil
}

// FindRunningRunForSession correlates a provider elicitation with a Matrix run
// when the provider supplied the remote session ID. An ambiguous match returns
// no run ID rather than attributing an intervention to the wrong run.
func (s *Store) FindRunningRunForSession(agentID, sessionID string) string {
	if agentID == "" || sessionID == "" {
		return ""
	}
	keys, err := s.storage.List(runKeyPrefix)
	if err != nil {
		return ""
	}
	match := ""
	for _, key := range keys {
		run, found, err := s.LoadRun(strings.TrimPrefix(key, runKeyPrefix))
		if err != nil || !found || run.Status != StatusRunning || run.AgentID != agentID || run.RemoteSessionID != sessionID {
			continue
		}
		if match != "" {
			return ""
		}
		match = run.ID
	}
	return match
}

func isWakeupEvent(kind string) bool {
	switch kind {
	case "run.completed", "run.failed", "run.cancelled", "run.outcome_unknown":
		return true
	default:
		return false
	}
}
