package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/google/uuid"
)

const (
	forkJobKeyPrefix = "session.forkjob."

	forkJobStatusQueued    = "queued"
	forkJobStatusRunning   = "running"
	forkJobStatusCompleted = "completed"
	forkJobStatusFailed    = "failed"
)

type forkJobRun struct {
	Req           middleware.SessionActionRequest
	ChildMeta     SessionMeta
	CleanupPolicy string
	JobID         string
}

//nolint:nilerr // Fork failures are protocol-visible typed results, not Go transport errors.
func (m *Manager) acceptAsyncFork(ctx context.Context, req middleware.SessionActionRequest, plan forkPlan, childMeta SessionMeta) (middleware.SessionActionResult, error) {
	activeID, parentRestored, err := m.restoreForkParent(req.ChannelID, plan, childMeta.ID)
	if err != nil {
		return m.forkWorkflowFailedResult(forkWorkflowFailureData{
			ChannelID: req.ChannelID,
			Plan:      plan,
			ChildMeta: childMeta,
			Err: forkWorkflowError{
				code:    "fork_parent_restore_failed",
				message: "failed to restore parent session after fork",
				err:     err,
			},
		}), nil
	}
	job := newForkJob(plan, childMeta, parentRestored)
	if err := m.saveForkJob(job); err != nil {
		return middleware.SessionActionResult{}, err
	}
	m.startForkJob(context.WithoutCancel(ctx), forkJobRun{
		Req:           req,
		ChildMeta:     childMeta,
		CleanupPolicy: plan.CleanupPolicy,
		JobID:         job.JobID,
	})
	childActive := activeID == childMeta.ID
	session := m.forkSessionEntry(childMeta, childActive, nil)
	return forkActionResult(forkResultData{
		ChildMeta:      childMeta,
		Session:        session,
		Plan:           plan,
		ActiveID:       activeID,
		ParentRestored: parentRestored,
		Async:          true,
		Job:            &job,
	}), nil
}

func newForkJob(plan forkPlan, childMeta SessionMeta, parentRestored bool) middleware.SessionForkJob {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return middleware.SessionForkJob{
		JobID:                  "forkjob-" + uuid.New().String(),
		Status:                 forkJobStatusQueued,
		ParentLogicalSessionID: plan.Parent.ID,
		ChildLogicalSessionID:  childMeta.ID,
		ParentRestored:         parentRestored,
		AcceptedAt:             now,
	}
}

func (m *Manager) startForkJob(ctx context.Context, run forkJobRun) {
	go m.runForkJob(ctx, run)
}

func (m *Manager) runForkJob(ctx context.Context, run forkJobRun) {
	job, found, err := m.loadForkJob(run.JobID)
	if err != nil || !found {
		return
	}
	job.Status = forkJobStatusRunning
	job.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_ = m.saveForkJob(job)

	artifact, cleanup, runErr := m.runForkChildWorkflow(ctx, run.Req, run.ChildMeta, run.CleanupPolicy)
	job.Artifact = artifact
	job.Cleanup = cleanup
	job.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if runErr != nil {
		code, message := forkWorkflowErrorCode(runErr)
		job.Status = forkJobStatusFailed
		job.Error = &middleware.SessionActionError{
			Code:    code,
			Message: message,
			Target:  run.ChildMeta.ID,
		}
	} else {
		job.Status = forkJobStatusCompleted
	}
	_ = m.saveForkJob(job)
}

func (m *Manager) handleSessionForkStatusTyped(req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	jobID := strings.TrimSpace(req.Target)
	if jobID == "" {
		return middleware.SessionActionResult{
			Action:      "fork_status",
			Unsupported: true,
			Error: &middleware.SessionActionError{
				Code:    "fork_job_not_found",
				Message: "fork job id is required",
			},
		}, nil
	}
	job, found, err := m.loadForkJob(jobID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if !found {
		return middleware.SessionActionResult{
			Action:      "fork_status",
			Unsupported: true,
			Error: &middleware.SessionActionError{
				Code:    "fork_job_not_found",
				Message: "fork job not found",
				Target:  jobID,
			},
		}, nil
	}
	return middleware.SessionActionResult{
		Action:  "fork_status",
		Message: fmt.Sprintf("Fork job %s: %s", job.JobID, job.Status),
		Fork: &middleware.SessionForkResult{
			ParentLogicalSessionID: job.ParentLogicalSessionID,
			ChildLogicalSessionID:  job.ChildLogicalSessionID,
			Async:                  true,
			JobID:                  job.JobID,
			Job:                    &job,
			Artifact:               job.Artifact,
			Cleanup:                job.Cleanup,
			ParentRestored:         job.ParentRestored,
		},
	}, nil
}

func (m *Manager) saveForkJob(job middleware.SessionForkJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return m.storage.Set(forkJobKey(job.JobID), data)
}

func (m *Manager) loadForkJob(jobID string) (middleware.SessionForkJob, bool, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return middleware.SessionForkJob{}, false, nil
	}
	data, err := m.storage.Get(forkJobKey(jobID))
	if err != nil {
		return middleware.SessionForkJob{}, false, err
	}
	if len(data) == 0 {
		return middleware.SessionForkJob{}, false, nil
	}
	var job middleware.SessionForkJob
	if err := json.Unmarshal(data, &job); err != nil {
		return middleware.SessionForkJob{}, false, err
	}
	return job, true, nil
}

func forkJobKey(jobID string) string {
	return forkJobKeyPrefix + jobID
}
