package session

import (
	"strings"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

const maxForkCleanupDepth = 8

// cleanupVisited records the sessions already handled in one recursive cleanup.

type cleanupVisited struct {
	seen map[string]struct{}
}

func newCleanupVisited() *cleanupVisited {
	return &cleanupVisited{seen: map[string]struct{}{}}
}

// enter records a session and reports whether it had not been seen yet. Both the
// logical id and the remote id are tracked, because the same provider session
// can appear under different logical names in a fork graph.

func (v *cleanupVisited) enter(meta SessionMeta) bool {
	if v == nil {
		return true
	}
	keys := make([]string, 0, 2)
	if id := strings.TrimSpace(meta.ID); id != "" {
		keys = append(keys, "logical:"+id)
	}
	if remote := strings.TrimSpace(meta.AgentSessionID); remote != "" {
		keys = append(keys, "remote:"+remote)
	}
	if len(keys) == 0 {
		return true
	}
	for _, key := range keys {
		if _, ok := v.seen[key]; ok {
			return false
		}
	}
	for _, key := range keys {
		v.seen[key] = struct{}{}
	}
	return true
}

// prepareCleanupExecution initialises the recursion guards on the outermost call.

func (m *Manager) prepareCleanupExecution(req sessionCleanupExecution) sessionCleanupExecution {
	if req.Visited == nil {
		req.Visited = newCleanupVisited()
	}
	return req
}

// cleanupAlreadyHandledResult reports a session that this same recursive cleanup
// already dealt with, or refused to enter because the graph was too deep.

func cleanupAlreadyHandledResult(req sessionCleanupExecution, policy string, tooDeep bool) middleware.SessionCleanupResult {
	result := middleware.SessionCleanupResult{
		LogicalSessionID: req.Meta.ID,
		RemoteSessionID:  req.Meta.AgentSessionID,
		AgentID:          req.Meta.AgentID,
		ProtocolKind:     req.Meta.ProtocolKind,
		CleanupPolicy:    policy,
		Clean:            true,
		StrongCleanup:    true,
		CleanupStrength:  sessioncleanup.StrengthStrong,
	}
	if tooDeep {
		// Reaching the depth limit means the graph was abandoned, not handled.
		result.Clean = false
		result.StrongCleanup = false
		result.CleanupStrength = sessioncleanup.StrengthFailed
		result.Error = "fork cleanup depth limit reached"
	}
	result.Warnings = sessioncleanup.AppendWarning(result.Warnings, sessioncleanup.WarningForkCleanupCycleSkipped)
	return result
}
