package runapi

import (
	"context"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

type runRelatedSessionRecorder struct {
	server     *Server
	ctx        context.Context
	scope      runCleanupScope
	cleanup    *middleware.SessionCleanupResult
	beforeSet  map[string]struct{}
	seen       map[string]struct{}
	cleanupErr error
}

func newRunRelatedSessionRecorder(ctx context.Context, server *Server, scope runCleanupScope, cleanup *middleware.SessionCleanupResult) *runRelatedSessionRecorder {
	beforeSet := sessionSnapshotSet(scope.beforeList)
	addSessionSnapshot(beforeSet, scope.before)
	return &runRelatedSessionRecorder{
		server:    server,
		ctx:       ctx,
		scope:     scope,
		cleanup:   cleanup,
		beforeSet: beforeSet,
		seen:      map[string]struct{}{},
	}
}

func (r *runRelatedSessionRecorder) record(snapshot sessionSnapshot, active bool) {
	key := sessionSnapshotKey(snapshot)
	if key == "" {
		return
	}
	if _, ok := r.seen[key]; ok {
		return
	}
	r.seen[key] = struct{}{}

	_, existedBefore := r.beforeSet[key]
	if !existedBefore && snapshotOwnedByRun(snapshot) {
		r.cleanupOwned(snapshot, active)
		return
	}
	markRunRelatedSessionRetained(r.cleanup, snapshot, active, sessioncleanup.WarningRunRelatedSessionRetained)
	if r.cleanupErr == nil {
		r.cleanupErr = sessioncleanup.FailureError(r.cleanup, "")
	}
}

func (r *runRelatedSessionRecorder) cleanupOwned(snapshot sessionSnapshot, active bool) {
	result, err := r.server.cleanupRunSession(r.ctx, r.scope.exec, snapshot)
	if result != nil {
		r.server.appendCleanupEvent(r.scope.exec.runID, r.scope.exec.agentID, *result)
	}
	if err == nil && result != nil && result.Clean {
		r.cleanup.RelatedSessions = append(r.cleanup.RelatedSessions, relatedSession(snapshot, active, false, "run_related_session_cleaned"))
		return
	}
	if err != nil && r.cleanupErr == nil {
		r.cleanupErr = err
	}
	r.cleanup.Warnings = sessioncleanup.AppendWarning(r.cleanup.Warnings, sessioncleanup.WarningRunRelatedSessionCleanupFailed)
	markRunRelatedSessionRetained(r.cleanup, snapshot, active, "run_related_session_cleanup_failed")
}

func sessionSnapshotSet(snapshots []sessionSnapshot) map[string]struct{} {
	out := map[string]struct{}{}
	for _, snapshot := range snapshots {
		addSessionSnapshot(out, snapshot)
	}
	return out
}

func addSessionSnapshot(set map[string]struct{}, snapshot sessionSnapshot) {
	if key := sessionSnapshotKey(snapshot); key != "" {
		set[key] = struct{}{}
	}
}

func sessionSnapshotKey(snapshot sessionSnapshot) string {
	if id := strings.TrimSpace(snapshot.LogicalSessionID); id != "" {
		return "logical:" + id
	}
	if id := strings.TrimSpace(snapshot.RemoteSessionID); id != "" {
		return "remote:" + id
	}
	return ""
}

func sameSessionSnapshot(a, b sessionSnapshot) bool {
	aLogical := strings.TrimSpace(a.LogicalSessionID)
	bLogical := strings.TrimSpace(b.LogicalSessionID)
	if aLogical != "" && bLogical != "" {
		return aLogical == bLogical
	}
	aRemote := strings.TrimSpace(a.RemoteSessionID)
	bRemote := strings.TrimSpace(b.RemoteSessionID)
	return aRemote != "" && bRemote != "" && aRemote == bRemote
}

func snapshotOwnedByRun(snapshot sessionSnapshot) bool {
	return snapshot.Ephemeral || strings.TrimSpace(snapshot.CleanupPolicy) != ""
}

func markRunRelatedSessionRetained(cleanup *middleware.SessionCleanupResult, snapshot sessionSnapshot, active bool, reason string) {
	cleanup.RelatedSessions = append(cleanup.RelatedSessions, relatedSession(snapshot, active, true, reason))
	cleanup.ProcessRetained = true
	cleanup.ProcessRetentionAllowed = true
	cleanup.ProcessRetentionReason = reason
	if cleanup.Clean {
		cleanup.Clean = false
		cleanup.StrongCleanup = false
		cleanup.CleanupStrength = sessioncleanup.StrengthFailed
		cleanup.WeakCleanupReason = sessioncleanup.WeakCleanupProcessRetained
	}
	if cleanup.FailureCode == "" {
		cleanup.FailureCode = sessioncleanup.FailureRunRelatedSessionRetained
	}
	if cleanup.Error == "" {
		cleanup.Error = "run cleanup retained a related session"
	}
	cleanup.Warnings = sessioncleanup.AppendWarning(cleanup.Warnings, reason)
}

func relatedSession(snapshot sessionSnapshot, active, retained bool, reason string) middleware.SessionCleanupRelatedSession {
	return middleware.SessionCleanupRelatedSession{
		LogicalSessionID: strings.TrimSpace(snapshot.LogicalSessionID),
		RemoteSessionID:  strings.TrimSpace(snapshot.RemoteSessionID),
		AgentID:          strings.TrimSpace(snapshot.AgentID),
		ProtocolKind:     strings.TrimSpace(snapshot.Protocol),
		WorkspaceID:      strings.TrimSpace(snapshot.WorkspaceID),
		WorkspacePath:    strings.TrimSpace(snapshot.WorkspacePath),
		ParentSessionID:  strings.TrimSpace(snapshot.ParentSessionID),
		ParentRemoteID:   strings.TrimSpace(snapshot.ParentRemoteID),
		Ephemeral:        snapshot.Ephemeral,
		CleanupPolicy:    strings.TrimSpace(snapshot.CleanupPolicy),
		Active:           active || snapshot.Active,
		Retained:         retained,
		Reason:           strings.TrimSpace(reason),
	}
}
