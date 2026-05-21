package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
)

func (m *Manager) cleanupForkChildren(ctx context.Context, req sessionCleanupExecution, policy string, result *middleware.SessionCleanupResult) {
	children, err := m.forkChildMetas(req.Meta)
	if err != nil {
		result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "fork_child_refs", err)
		return
	}
	for _, child := range children {
		childCleanup := m.cleanupSessionMirrorAndRemote(ctx, sessionCleanupExecution{
			ChannelID:                      req.ChannelID,
			Meta:                           child,
			CleanupPolicy:                  policy,
			ForceForgetLocal:               true,
			SuppressForkParentOwnerCleanup: true,
		})
		result.ForkChildren = append(result.ForkChildren, childCleanup)
		result.ForkChildrenCleaned = len(result.ForkChildren)
	}
}

func (m *Manager) markForkChildCleanupErrors(result *middleware.SessionCleanupResult) {
	for _, childCleanup := range result.ForkChildren {
		if childCleanup.Clean {
			continue
		}
		target := firstNonEmpty(childCleanup.LogicalSessionID, childCleanup.RemoteSessionID)
		err := fmt.Errorf("fork child %s cleanup failed", target)
		if strings.TrimSpace(childCleanup.Error) != "" {
			err = fmt.Errorf("%w: %s", err, childCleanup.Error)
		}
		result.Error, result.FailureCode = sessioncleanup.AppendErrorWithCode(result.Error, result.FailureCode, "fork_child_cleanup", err)
	}
}

func (m *Manager) forkChildMetas(parent SessionMeta) ([]SessionMeta, error) {
	keys, err := m.storage.List("session.meta.")
	if err != nil {
		return nil, err
	}
	parentID := strings.TrimSpace(parent.ID)
	parentRemoteID := strings.TrimSpace(parent.AgentSessionID)
	children := []SessionMeta{}
	for _, key := range keys {
		raw, err := m.storage.Get(key)
		if err != nil || len(raw) == 0 {
			continue
		}
		var meta SessionMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			continue
		}
		if meta.ID == parent.ID {
			continue
		}
		if strings.TrimSpace(meta.ParentSessionID) == parentID ||
			parentRemoteID != "" && strings.TrimSpace(meta.ParentRemoteID) == parentRemoteID {
			children = append(children, meta)
		}
	}
	return children, nil
}

func forkChildrenClean(children []middleware.SessionCleanupResult) bool {
	for _, child := range children {
		if !child.Clean {
			return false
		}
	}
	return true
}
