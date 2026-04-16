package runtrace

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Start creates a canonical run record and its initial run.started event.
func (s *Store) Start(run Run) (Run, Event, error) {
	if s == nil || s.storage == nil {
		return Run{}, Event{}, fmt.Errorf("run trace storage not available")
	}
	run = normalizeNewRun(run)
	if err := s.SaveRun(run); err != nil {
		return Run{}, Event{}, err
	}
	event, err := s.AppendEvent(startedEvent(run))
	if err != nil {
		return Run{}, Event{}, err
	}
	return run, event, nil
}

func normalizeNewRun(run Run) Run {
	now := time.Now().UTC()
	run.ID = strings.TrimSpace(run.ID)
	if run.ID == "" {
		run.ID = "run-" + uuid.NewString()
	}
	run.AgentID = strings.TrimSpace(run.AgentID)
	run.ChannelID = strings.TrimSpace(run.ChannelID)
	run.ExecutionMode = normalizeExecutionMode(run.ExecutionMode)
	if run.Status == "" {
		run.Status = StatusRunning
	}
	if run.InputKind == "" {
		run.InputKind = "user_message"
	}
	if isEmptyTracePolicy(run.TracePolicy) {
		run.TracePolicy.IncludeProtocolMeta = true
	}
	if run.TracePolicy.ContentMode == "" {
		run.TracePolicy.ContentMode = ContentModeRefs
	}
	if run.TracePolicy.RedactionProfile == "" {
		run.TracePolicy.RedactionProfile = "default"
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = now
	}
	run.UpdatedAt = now
	if run.InputDigest == "" && run.InputRef != "" {
		run.InputDigest = DigestString(run.InputRef)
	}
	return run
}

func isEmptyTracePolicy(policy TracePolicy) bool {
	return policy.ContentMode == "" && policy.RedactionProfile == "" && !policy.IncludeProtocolMeta
}

func startedEvent(run Run) Event {
	return Event{
		RunID:     run.ID,
		Kind:      "run.started",
		Actor:     "matrix",
		Status:    StatusCompleted,
		Timestamp: run.StartedAt,
		Metadata: map[string]interface{}{
			"execution_mode": run.ExecutionMode,
			"channel_id":     run.ChannelID,
			"workspace_id":   run.WorkspaceID,
		},
	}
}

func (s *Store) Complete(runID, output, stopReason string) (Run, error) {
	run, found, err := s.LoadRun(runID)
	if err != nil {
		return Run{}, err
	}
	if !found {
		return Run{}, fmt.Errorf("run %s not found", runID)
	}
	if run.Status == StatusCancelled {
		return run, nil
	}
	run = completeRun(run, output, stopReason)
	if err := s.SaveRun(run); err != nil {
		return Run{}, err
	}
	if err := s.appendFinalMessage(run, output); err != nil {
		return Run{}, err
	}
	_, err = s.AppendEvent(Event{RunID: run.ID, Kind: "run.completed", Actor: "matrix", Status: StatusCompleted, Timestamp: run.CompletedAt})
	return run, err
}

func completeRun(run Run, output, stopReason string) Run {
	now := time.Now().UTC()
	run.Status = StatusCompleted
	run.StopReason = firstNonEmpty(stopReason, "end_turn")
	run.Output = output
	run.OutputRef = "matrix://runs/" + run.ID + "/outcome"
	run.OutputDigest = DigestString(output)
	run.CompletedAt = now
	run.UpdatedAt = now
	return run
}

func (s *Store) appendFinalMessage(run Run, output string) error {
	if output == "" {
		return nil
	}
	_, err := s.AppendEvent(Event{
		RunID:         run.ID,
		Kind:          "agent.message.final",
		Actor:         run.AgentID,
		Status:        StatusCompleted,
		Timestamp:     run.CompletedAt,
		Protocol:      run.Protocol,
		ContentRef:    run.OutputRef,
		ContentDigest: run.OutputDigest,
		ProtocolMeta:  map[string]interface{}{"execution_mode": run.ExecutionMode},
	})
	return err
}

func (s *Store) Fail(runID string, runErr error) (Run, error) {
	run, found, err := s.LoadRun(runID)
	if err != nil {
		return Run{}, err
	}
	if !found {
		return Run{}, fmt.Errorf("run %s not found", runID)
	}
	if run.Status == StatusCancelled {
		return run, nil
	}
	run = failRun(run, runErr)
	if err := s.SaveRun(run); err != nil {
		return Run{}, err
	}
	_, err = s.AppendEvent(Event{RunID: run.ID, Kind: "run.failed", Actor: "matrix", Status: StatusFailed, Timestamp: run.CompletedAt, Message: run.Error})
	return run, err
}

func failRun(run Run, runErr error) Run {
	now := time.Now().UTC()
	run.Status = StatusFailed
	run.StopReason = "error"
	if runErr != nil {
		run.Error = runErr.Error()
	}
	run.CompletedAt = now
	run.UpdatedAt = now
	return run
}

func (s *Store) Cancel(runID, reason string) (Run, error) {
	run, found, err := s.LoadRun(runID)
	if err != nil {
		return Run{}, err
	}
	if !found {
		return Run{}, fmt.Errorf("run %s not found", runID)
	}
	if run.Status != "" && run.Status != StatusRunning {
		return run, nil
	}
	run = cancelRun(run, reason)
	if err := s.SaveRun(run); err != nil {
		return Run{}, err
	}
	_, err = s.AppendEvent(Event{RunID: run.ID, Kind: "run.cancelled", Actor: "matrix", Status: StatusCancelled, Timestamp: run.CompletedAt, Message: reason})
	return run, err
}

func cancelRun(run Run, reason string) Run {
	now := time.Now().UTC()
	run.Status = StatusCancelled
	run.StopReason = firstNonEmpty(reason, "cancelled")
	run.CompletedAt = now
	run.UpdatedAt = now
	return run
}
