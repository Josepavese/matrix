package runapi

import "github.com/Josepavese/matrix/internal/middleware"

func cleanupLogArgs(cleanup *middleware.SessionCleanupResult) []any {
	if cleanup == nil {
		return nil
	}
	return []any{
		"cleanup_clean", cleanup.Clean,
		"strong_cleanup", cleanup.StrongCleanup,
		"cleanup_strength", cleanup.CleanupStrength,
		"remote_canceled", cleanup.RemoteCanceled,
		"process_reaped", cleanup.ProcessReaped,
		"warnings", cleanup.Warnings,
		"failure_code", cleanup.FailureCode,
	}
}
