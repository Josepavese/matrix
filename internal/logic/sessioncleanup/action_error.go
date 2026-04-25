package sessioncleanup

import (
	"log/slog"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

func ActionError(cleanup middleware.SessionCleanupResult, targetID string) *middleware.SessionActionError {
	return &middleware.SessionActionError{
		Code:    actionErrorCode(cleanup),
		Message: actionErrorMessage(cleanup),
		Target:  targetID,
	}
}

func LogTypedFailure(action, targetID string, cleanup middleware.SessionCleanupResult) {
	slog.Warn("matrix session cleanup returned typed failure",
		"action", action,
		"target", targetID,
		"agent_id", cleanup.AgentID,
		"logical_session_id", cleanup.LogicalSessionID,
		"remote_session_id", cleanup.RemoteSessionID,
		"failure_code", cleanup.FailureCode,
		"cleanup_strength", cleanup.CleanupStrength,
		"clean", cleanup.Clean,
		"strong_cleanup", cleanup.StrongCleanup,
		"local_forgotten", cleanup.LocalForgotten,
		"remote_deleted", cleanup.RemoteDeleted,
		"remote_closed", cleanup.RemoteClosed,
		"remote_canceled", cleanup.RemoteCanceled,
		"process_reaped", cleanup.ProcessReaped,
		"process_retained", cleanup.ProcessRetained,
		"error", cleanup.Error,
	)
}

func actionErrorCode(cleanup middleware.SessionCleanupResult) string {
	if code := strings.TrimSpace(cleanup.FailureCode); code != "" {
		return code
	}
	if cleanup.Clean {
		return "cleanup_warning"
	}
	return "cleanup_failed"
}

func actionErrorMessage(cleanup middleware.SessionCleanupResult) string {
	if message := strings.TrimSpace(cleanup.Error); message != "" {
		return message
	}
	return "cleanup did not reach a clean provider/process state"
}
