package runtrace

import "fmt"

// NotifyUnobservedActivity emits a durable, minimal supervisor wakeup. The
// transition lock prevents a late timer from waking a run that already ended.
// Silence is an observation, never a provider failure diagnosis.
func (s *Store) NotifyUnobservedActivity(runID string) error {
	lock := s.transitionLock(runID)
	lock.Lock()
	defer lock.Unlock()
	run, found, err := s.LoadRun(runID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("run %s not found", runID)
	}
	if isTerminalStatus(run.Status) {
		return nil
	}
	_, err = s.AppendEvent(Event{
		RunID: runID, Kind: "run.attention_required", Actor: "matrix", Status: run.Status,
		Metadata: map[string]interface{}{"failure_code": "activity_unobserved", "cause": "unknown", "source": "matrix_activity_observer"},
	})
	return err
}
