package runtrace

import (
	"fmt"
	"strings"
	"time"
)

// RecoverInterruptedRuns marks records left active by a previous daemon as
// uncertain. It never sends a prompt or claims that remote work failed.
func (s *Store) RecoverInterruptedRuns() (int, error) {
	if s == nil || s.storage == nil {
		return 0, fmt.Errorf("run trace storage not available")
	}
	keys, err := s.storage.List(runKeyPrefix)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, key := range keys {
		runID := strings.TrimPrefix(key, runKeyPrefix)
		lock := s.transitionLock(runID)
		lock.Lock()
		run, found, err := s.LoadRun(runID)
		if err != nil {
			lock.Unlock()
			return count, err
		}
		if !found || isTerminalStatus(run.Status) {
			lock.Unlock()
			continue
		}
		now := time.Now().UTC()
		run.Status = StatusUnknown
		run.StopReason = "daemon_interrupted"
		run.Error = "daemon restarted before the remote outcome was verified; do not replay automatically"
		run.CompletedAt = now
		run.UpdatedAt = now
		if err := s.SaveRun(run); err != nil {
			lock.Unlock()
			return count, err
		}
		_, err = s.AppendEvent(Event{RunID: runID, Kind: "run.outcome_unknown", Actor: "matrix", Status: StatusUnknown, Timestamp: now, Metadata: map[string]interface{}{"failure_code": "daemon_interrupted"}})
		lock.Unlock()
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
