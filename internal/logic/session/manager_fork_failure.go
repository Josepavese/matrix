package session

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

type forkWorkflowError struct {
	code    string
	message string
	err     error
}

func (e forkWorkflowError) Error() string {
	if e.err == nil {
		return e.message
	}
	return e.err.Error()
}

func (e forkWorkflowError) Unwrap() error { return e.err }

type forkWorkflowFailureData struct {
	ChannelID string
	Plan      forkPlan
	ChildMeta SessionMeta
	Artifact  *middleware.SessionForkArtifact
	Cleanup   *middleware.SessionCleanupResult
	Err       error
}

func (m *Manager) forkWorkflowFailedResult(data forkWorkflowFailureData) middleware.SessionActionResult {
	activeID, parentRestored, err := m.restoreAfterForkFailure(data)
	code, message := forkWorkflowErrorCode(err)
	return middleware.SessionActionResult{
		Action:          "fork",
		Message:         message,
		ActiveSessionID: activeID,
		Error: &middleware.SessionActionError{
			Code:    code,
			Message: message,
			Target:  data.ChildMeta.ID,
		},
		Fork: &middleware.SessionForkResult{
			ParentLogicalSessionID: data.Plan.Parent.ID,
			ParentRemoteSessionID:  data.Plan.Parent.AgentSessionID,
			ChildLogicalSessionID:  data.ChildMeta.ID,
			Child:                  &data.Plan.ChildRemote,
			MakeActive:             data.Plan.MakeActive,
			Ephemeral:              data.ChildMeta.Ephemeral,
			CleanupPolicy:          data.Plan.CleanupPolicy,
			Artifact:               data.Artifact,
			Cleanup:                data.Cleanup,
			ParentRestored:         parentRestored,
			Reason:                 errorString(err),
		},
	}
}

func (m *Manager) restoreAfterForkFailure(data forkWorkflowFailureData) (string, bool, error) {
	activeID, parentRestored, restoreErr := m.restoreForkParent(data.ChannelID, data.Plan, data.ChildMeta.ID)
	if restoreErr == nil {
		return activeID, parentRestored, data.Err
	}
	if data.Err == nil {
		return activeID, parentRestored, restoreErr
	}
	return activeID, parentRestored, errors.Join(data.Err, fmt.Errorf("parent restore failed: %w", restoreErr))
}

func forkWorkflowErrorCode(err error) (string, string) {
	code := "fork_child_turn_failed"
	message := "fork child artifact turn failed"
	var workflowErr forkWorkflowError
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		if ok := errorAsForkWorkflow(err, &workflowErr); ok {
			code = workflowErr.code
			message = workflowErr.message
		}
	}
	if strings.Contains(errorString(err), "parent restore failed") {
		return "fork_parent_restore_failed", "failed to restore parent session after fork"
	}
	return code, message
}

func errorAsForkWorkflow(err error, target *forkWorkflowError) bool { return errors.As(err, target) }

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
