package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

const forkCleanupTimeout = 30 * time.Second

func (m *Manager) runForkChildWorkflow(ctx context.Context, req middleware.SessionActionRequest, childMeta SessionMeta, cleanupPolicy string) (*middleware.SessionForkArtifact, *middleware.SessionCleanupResult, error) {
	artifact, err := m.runForkChildTurn(ctx, req, childMeta)
	if err != nil {
		cleanup, cleanupErr := m.cleanupForkChildIfRequested(ctx, req, childMeta.ID, cleanupPolicy)
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
	cleanup, err := m.cleanupForkChild(ctx, req, childMeta.ID, cleanupPolicy)
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

func (m *Manager) cleanupForkChild(ctx context.Context, req middleware.SessionActionRequest, childID string, cleanupPolicy string) (*middleware.SessionCleanupResult, error) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forkCleanupTimeout)
	defer cancel()
	result, err := m.cleanupSessionTyped(cleanupCtx, sessionCleanupRequest{
		ChannelID:        req.ChannelID,
		Lang:             m.wizard.GetLanguage(req.ChannelID),
		Target:           childID,
		Action:           "cleanup",
		CleanupPolicy:    cleanupPolicy,
		ForceForgetLocal: true,
	})
	if err != nil {
		return result.Cleanup, err
	}
	return result.Cleanup, nil
}

func (m *Manager) cleanupForkChildIfRequested(ctx context.Context, req middleware.SessionActionRequest, childID string, cleanupPolicy string) (*middleware.SessionCleanupResult, error) {
	if !forkCleanupRequested(req) {
		return nil, nil
	}
	return m.cleanupForkChild(ctx, req, childID, cleanupPolicy)
}

func forkCleanupRequested(req middleware.SessionActionRequest) bool {
	return req.Ephemeral || strings.TrimSpace(req.CleanupPolicy) != ""
}
