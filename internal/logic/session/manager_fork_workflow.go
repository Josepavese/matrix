package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

const forkCleanupTimeout = 30 * time.Second

func (m *Manager) runForkChildWorkflow(ctx context.Context, req middleware.SessionActionRequest, childMeta SessionMeta, cleanupPolicy string) (*middleware.SessionForkArtifact, *middleware.SessionCleanupResult, error) {
	artifact, err := m.runForkChildTurn(ctx, req, childMeta)
	if err != nil {
		cleanup, cleanupErr := m.cleanupForkChildIfRequested(ctx, req, childMeta, cleanupPolicy)
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("fork child cleanup failed: %w", cleanupErr))
		}
		return artifact, cleanup, forkWorkflowError{
			code:    "fork_child_turn_failed",
			message: "fork child artifact turn failed",
			err:     err,
		}
	}
	if artifact == nil || !forkCleanupRequested(req) {
		return artifact, nil, nil
	}
	cleanup, err := m.cleanupForkChild(ctx, req, childMeta, cleanupPolicy)
	if err == nil {
		return artifact, cleanup, nil
	}
	return artifact, cleanup, forkWorkflowError{
		code:    "fork_child_cleanup_failed",
		message: "fork child cleanup failed",
		err:     err,
	}
}

func (m *Manager) runForkChildTurn(ctx context.Context, req middleware.SessionActionRequest, childMeta SessionMeta) (*middleware.SessionForkArtifact, error) {
	input := strings.TrimSpace(req.Input)
	if input == "" {
		return nil, nil
	}
	output, err := m.routeResolvedSession(ctx, middleware.ConversationRequest{
		ChannelID:        req.ChannelID,
		AgentID:          childMeta.AgentID,
		LogicalSessionID: childMeta.ID,
		WorkspaceID:      childMeta.WorkspaceID,
		WorkspacePath:    childMeta.WorkspacePath,
		Input:            input,
		NonInteractive:   true,
	}, childMeta.ID, childMeta.AgentID)
	if err != nil {
		return nil, err
	}
	return &middleware.SessionForkArtifact{Kind: "child_turn_response", Content: output}, nil
}

func (m *Manager) cleanupForkChild(ctx context.Context, req middleware.SessionActionRequest, childMeta SessionMeta, cleanupPolicy string) (*middleware.SessionCleanupResult, error) {
	if _, found, err := m.loadSessionMeta(childMeta.ID); err == nil && !found {
		return alreadyCleanedForkChild(childMeta.ID, cleanupPolicy), nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forkCleanupTimeout)
	defer cancel()
	result, err := m.cleanupSessionTyped(cleanupCtx, sessionCleanupRequest{
		ChannelID:                      req.ChannelID,
		Lang:                           m.wizard.GetLanguage(req.ChannelID),
		Target:                         childMeta.ID,
		Action:                         "cleanup",
		CleanupPolicy:                  cleanupPolicy,
		ForceForgetLocal:               true,
		SuppressForkParentOwnerCleanup: !m.canRemediateForkParentOwner(childMeta, cleanupPolicy),
	})
	if err != nil {
		return result.Cleanup, err
	}
	m.finalizeForkChildCleanupProof(childMeta, result.Cleanup)
	if result.Cleanup != nil && !result.Cleanup.Clean {
		return result.Cleanup, sessioncleanup.FailureError(result.Cleanup, "")
	}
	return result.Cleanup, nil
}

func (m *Manager) canRemediateForkParentOwner(childMeta SessionMeta, cleanupPolicy string) bool {
	parent, found := m.loadForkParentMeta(childMeta)
	return found && forkParentOwnerCleanupAllowed(childMeta, parent, cleanupPolicy)
}

func alreadyCleanedForkChild(childID string, cleanupPolicy string) *middleware.SessionCleanupResult {
	policy := cleanupPolicy
	if strings.TrimSpace(policy) == "" {
		policy = middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal
	}
	return &middleware.SessionCleanupResult{
		LogicalSessionID:  childID,
		CleanupPolicy:     policy,
		Clean:             true,
		CleanupStrength:   sessioncleanup.StrengthWeak,
		WeakCleanupReason: "already_cleaned",
		LocalForgotten:    true,
		Warnings:          []string{sessioncleanup.WarningForkChildCleanupAlreadyMissing},
	}
}

func (m *Manager) cleanupForkChildIfRequested(ctx context.Context, req middleware.SessionActionRequest, childMeta SessionMeta, cleanupPolicy string) (*middleware.SessionCleanupResult, error) {
	if !forkCleanupRequested(req) {
		return nil, nil
	}
	return m.cleanupForkChild(ctx, req, childMeta, cleanupPolicy)
}

func forkCleanupRequested(req middleware.SessionActionRequest) bool {
	return req.Ephemeral || strings.TrimSpace(req.CleanupPolicy) != ""
}
