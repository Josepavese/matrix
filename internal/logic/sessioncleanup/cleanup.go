package sessioncleanup

import (
	"errors"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

const NoMatchingCachedAgentClient = "no matching cached agent client"
const OtherLocalSessionsStillReferenceAgentClient = "other local sessions still reference agent client"
const FailureAgentStartContextCancelledDuringCleanup = "agent_start_context_cancelled_during_cleanup"
const NoReusableCachedAgentClient = "no reusable cached agent client"
const WarningRemoteLifecycleSkippedNoReusableClient = "remote_lifecycle_skipped_no_reusable_cached_agent_client"
const WarningRemoteCancelSessionNotFoundAfterProcessReap = "remote_cancel_session_not_found_after_process_reap"
const WarningForkChildCleanupAlreadyMissing = "fork_child_cleanup_already_missing"
const WarningRunRelatedSessionRetained = "run_related_session_retained"
const WarningRunRelatedSessionCleanupFailed = "run_related_session_cleanup_failed"
const WarningRunAgentClientReconcileFailed = "run_agent_client_reconcile_failed"
const ReasonRunUnreferencedAgentClientReaped = "run_unreferenced_agent_client_reaped"
const ReasonForkParentAgentClientOwner = "fork_parent_agent_client_owner"
const ReasonSharedAgentClientOwner = "shared_agent_client_owner"
const FailureRunRelatedSessionRetained = WarningRunRelatedSessionRetained
const ForkChildUsesParentAgentClient = "fork child uses parent agent client"
const WeakCleanupNoRemoteOrProcessProof = "cleanup_clean_without_remote_or_process_proof"
const WeakCleanupProcessRetained = "process_retained"

const (
	StrengthStrong   = "strong"
	StrengthWeak     = "weak"
	StrengthRetained = "retained"
	StrengthFailed   = "failed"
)

type CleanInput struct {
	Ephemeral               bool
	RemoteSessionID         string
	CleanupPolicy           string
	RemoteDeleted           bool
	RemoteClosed            bool
	RemoteCanceled          bool
	ProcessReapRequired     bool
	ProcessReaped           bool
	ProcessAbsent           bool
	ProcessAbsenceReason    string
	ProcessRetained         bool
	ProcessRetentionAllowed bool
	ProcessRetentionReason  string
	LocalForgotten          bool
}

func NormalizePolicy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case middleware.SessionCleanupPolicyDeleteRemote:
		return middleware.SessionCleanupPolicyDeleteRemote
	case middleware.SessionCleanupPolicyForgetLocal:
		return middleware.SessionCleanupPolicyForgetLocal
	case middleware.SessionCleanupPolicyDeleteRemoteOrForgetLocal,
		middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
		middleware.SessionCleanupPolicyDeleteRemoteOrCancelForgetLocalAlias:
		return middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal
	default:
		return middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal
	}
}

func AllowsLocalForget(policy string) bool {
	return policy != middleware.SessionCleanupPolicyDeleteRemote
}

func AllowsCancelFallback(policy string) bool {
	return policy == middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal
}

func ShouldForgetLocalMirror(policy string, forceForgetLocal bool, remoteDeleted bool) bool {
	return forceForgetLocal || remoteDeleted || AllowsLocalForget(policy)
}

func IsRemoteDeleteUnsupported(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not advertise session/delete") ||
		strings.Contains(msg, "does not expose remote session control") ||
		strings.Contains(msg, "delete") && strings.Contains(msg, "unsupported")
}

func IsRemoteCloseUnsupported(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not advertise session/close") ||
		strings.Contains(msg, "does not expose remote session close") ||
		strings.Contains(msg, "close") && strings.Contains(msg, "unsupported")
}

func IsNoReusableCachedAgentClient(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), NoReusableCachedAgentClient)
}

func AppendError(existing, phase string, err error) string {
	if err == nil {
		return existing
	}
	msg := phase + ": " + err.Error()
	if existing == "" {
		return msg
	}
	return existing + "; " + msg
}

func AppendErrorWithCode(existing, code, phase string, err error) (string, string) {
	if err == nil {
		return existing, code
	}
	existing = AppendError(existing, phase, err)
	if code == "" {
		code = FailureCode(err)
		if code == "" {
			code = strings.TrimSpace(phase)
		}
	}
	return existing, code
}

func FailureError(cleanup *middleware.SessionCleanupResult, fallback string) error {
	reason := FailureReason(cleanup)
	if reason == "" {
		reason = strings.TrimSpace(fallback)
	}
	if reason == "" {
		reason = "not clean"
	}
	return errors.New("session cleanup failed: " + reason)
}

func FailureReason(cleanup *middleware.SessionCleanupResult) string {
	if cleanup == nil {
		return ""
	}
	if code := strings.TrimSpace(cleanup.FailureCode); code != "" {
		return code
	}
	return strings.TrimSpace(cleanup.Error)
}

func AppendWarning(existing []string, warning string) []string {
	warning = strings.TrimSpace(warning)
	if warning == "" || HasWarning(existing, warning) {
		return existing
	}
	return append(existing, warning)
}

func HasWarning(warnings []string, warning string) bool {
	for _, existing := range warnings {
		if existing == warning {
			return true
		}
	}
	return false
}

func FailureCode(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	contextCancelled := strings.Contains(msg, "context canceled") || strings.Contains(msg, "context cancelled")
	if strings.Contains(msg, "failed to start agent") && contextCancelled {
		return FailureAgentStartContextCancelledDuringCleanup
	}
	return ""
}

func IsClean(input CleanInput) bool {
	if !input.LocalForgotten || !remoteCleanupSatisfied(input) || !processCleanupSatisfied(input) {
		return false
	}
	if input.Ephemeral && !HasStrongProof(input) {
		return false
	}
	return true
}

func remoteCleanupSatisfied(input CleanInput) bool {
	return strings.TrimSpace(input.RemoteSessionID) == "" ||
		input.RemoteDeleted ||
		input.RemoteClosed ||
		input.RemoteCanceled ||
		input.ProcessReaped ||
		input.CleanupPolicy == middleware.SessionCleanupPolicyForgetLocal
}

func processCleanupSatisfied(input CleanInput) bool {
	return !input.ProcessReapRequired ||
		input.ProcessReaped ||
		input.ProcessAbsent ||
		input.ProcessRetained && input.ProcessRetentionAllowed ||
		!input.ProcessRetained && input.ProcessRetentionReason == NoMatchingCachedAgentClient
}

func HasStrongProof(input CleanInput) bool {
	return input.RemoteDeleted || input.RemoteClosed || input.RemoteCanceled || input.ProcessReaped || processAbsenceStrong(input)
}

func processAbsenceStrong(input CleanInput) bool {
	if !input.ProcessAbsent || strings.TrimSpace(input.RemoteSessionID) != "" {
		return false
	}
	reason := strings.TrimSpace(input.ProcessAbsenceReason)
	if reason == "" {
		reason = strings.TrimSpace(input.ProcessRetentionReason)
	}
	return reason == NoMatchingCachedAgentClient
}

func Strength(input CleanInput) string {
	if !IsClean(input) {
		return StrengthFailed
	}
	if input.ProcessRetained && input.ProcessRetentionAllowed {
		return StrengthRetained
	}
	if HasStrongProof(input) {
		return StrengthStrong
	}
	return StrengthWeak
}

func WeakReason(input CleanInput) string {
	if input.ProcessRetained {
		return WeakCleanupProcessRetained
	}
	if HasStrongProof(input) {
		return ""
	}
	return WeakCleanupNoRemoteOrProcessProof
}

func Message(action string, cleanup middleware.SessionCleanupResult) string {
	verb := "Cleaned up"
	if action == "delete" {
		verb = "Deleted"
	}
	return verb + " session: " + cleanup.LogicalSessionID +
		" (remote_deleted=" + boolText(cleanup.RemoteDeleted) +
		" remote_closed=" + boolText(cleanup.RemoteClosed) +
		" remote_canceled=" + boolText(cleanup.RemoteCanceled) +
		" local_forgotten=" + boolText(cleanup.LocalForgotten) + ")"
}

func Metadata(cleanup middleware.SessionCleanupResult) map[string]interface{} {
	return map[string]interface{}{
		"logical_session_id":        cleanup.LogicalSessionID,
		"remote_session_id":         cleanup.RemoteSessionID,
		"agent_id":                  cleanup.AgentID,
		"protocol_kind":             cleanup.ProtocolKind,
		"cleanup_policy":            cleanup.CleanupPolicy,
		"clean":                     cleanup.Clean,
		"strong_cleanup":            cleanup.StrongCleanup,
		"cleanup_strength":          cleanup.CleanupStrength,
		"weak_cleanup_reason":       cleanup.WeakCleanupReason,
		"remote_delete_attempted":   cleanup.RemoteDeleteAttempted,
		"remote_deleted":            cleanup.RemoteDeleted,
		"remote_delete_unsupported": cleanup.RemoteDeleteUnsupported,
		"remote_close_attempted":    cleanup.RemoteCloseAttempted,
		"remote_closed":             cleanup.RemoteClosed,
		"remote_close_unsupported":  cleanup.RemoteCloseUnsupported,
		"remote_cancel_attempted":   cleanup.RemoteCancelAttempted,
		"remote_canceled":           cleanup.RemoteCanceled,
		"process_reap_attempted":    cleanup.ProcessReapAttempted,
		"process_reaped":            cleanup.ProcessReaped,
		"process_absent":            cleanup.ProcessAbsent,
		"process_absence_reason":    cleanup.ProcessAbsenceReason,
		"process_retained":          cleanup.ProcessRetained,
		"process_retention_allowed": cleanup.ProcessRetentionAllowed,
		"process_retention_reason":  cleanup.ProcessRetentionReason,
		"local_forgotten":           cleanup.LocalForgotten,
		"fork_children_cleaned":     cleanup.ForkChildrenCleaned,
		"fork_children":             cleanup.ForkChildren,
		"related_sessions":          cleanup.RelatedSessions,
		"warnings":                  cleanup.Warnings,
		"failure_code":              cleanup.FailureCode,
		"error":                     cleanup.Error,
	}
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
