package runtrace

import "fmt"

// UpdateRun serializes a metadata update with terminal transitions. Live
// identity/model callbacks must not save a stale running record over a cancel.
func (s *Store) UpdateRun(runID string, update func(*Run)) error {
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
	update(&run)
	return s.SaveRun(run)
}
