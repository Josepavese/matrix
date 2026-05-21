package session

import (
	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

func finalizeCleanupResult(meta SessionMeta, result *middleware.SessionCleanupResult) {
	appendProcessReapWarnings(result)
	cleanInput := cleanupResultInput(meta, result)
	childrenClean := forkChildrenClean(result.ForkChildren)
	applyCleanupProof(cleanInput, childrenClean, result)
	clearCleanFailureState(result)
}

func appendProcessReapWarnings(result *middleware.SessionCleanupResult) {
	if result.ProcessReaped && sessioncleanup.HasWarning(result.Warnings, sessioncleanup.WarningRemoteLifecycleSkippedNoReusableClient) {
		result.Warnings = sessioncleanup.AppendWarning(result.Warnings, sessioncleanup.WarningRemoteCancelSessionNotFoundAfterProcessReap)
	}
}

func applyCleanupProof(cleanInput sessioncleanup.CleanInput, childrenClean bool, result *middleware.SessionCleanupResult) {
	result.Clean = sessioncleanup.IsClean(cleanInput) && childrenClean
	result.CleanupStrength = sessioncleanup.Strength(cleanInput)
	result.StrongCleanup = result.Clean && result.CleanupStrength == sessioncleanup.StrengthStrong
	if !childrenClean {
		markForkChildCleanupFailed(result)
	}
	if result.Clean && !result.StrongCleanup {
		result.WeakCleanupReason = sessioncleanup.WeakReason(cleanInput)
	}
	if !result.Clean && result.FailureCode == "" && !sessioncleanup.HasStrongProof(cleanInput) {
		result.FailureCode = sessioncleanup.WeakReason(cleanInput)
	}
}

func clearCleanFailureState(result *middleware.SessionCleanupResult) {
	if !result.Clean {
		return
	}
	result.FailureCode = ""
	result.Error = ""
}

func cleanupResultInput(meta SessionMeta, result *middleware.SessionCleanupResult) sessioncleanup.CleanInput {
	return sessioncleanup.CleanInput{
		Ephemeral:               meta.Ephemeral,
		RemoteSessionID:         result.RemoteSessionID,
		CleanupPolicy:           result.CleanupPolicy,
		RemoteDeleted:           result.RemoteDeleted,
		RemoteClosed:            result.RemoteClosed,
		RemoteCanceled:          result.RemoteCanceled,
		ProcessReapRequired:     result.ProcessReapAttempted || result.ProcessRetained && !result.ProcessRetentionAllowed,
		ProcessReaped:           result.ProcessReaped,
		ProcessAbsent:           result.ProcessAbsent,
		ProcessAbsenceReason:    result.ProcessAbsenceReason,
		ProcessRetained:         result.ProcessRetained,
		ProcessRetentionAllowed: result.ProcessRetentionAllowed,
		ProcessRetentionReason:  result.ProcessRetentionReason,
		LocalForgotten:          result.LocalForgotten,
	}
}

func markForkChildCleanupFailed(result *middleware.SessionCleanupResult) {
	result.CleanupStrength = sessioncleanup.StrengthFailed
	if result.FailureCode == "" {
		result.FailureCode = "fork_child_cleanup"
	}
	if result.Error == "" {
		result.Error = "one or more fork child cleanups failed"
	}
}
