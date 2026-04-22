package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jose/matrix-v2/internal/middleware"
)

func (m *Manager) handleSessionCapabilitiesTyped(ctx context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	agentID, err := m.resolveActionAgentID(req)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	reporter, ok := m.router.(middleware.AgentCapabilityReporter)
	if !ok {
		return middleware.SessionActionResult{
			Action:      "capabilities",
			Message:     "Provider capability report is not available.",
			Unsupported: true,
		}, nil
	}
	report, err := reporter.AgentCapabilities(ctx, agentID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	return middleware.SessionActionResult{
		Action:       "capabilities",
		Message:      "Provider capabilities loaded.",
		Capabilities: &report,
	}, nil
}

func (m *Manager) handleSessionForkTyped(ctx context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	meta, err := m.resolveActionSession(req)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if strings.TrimSpace(meta.AgentSessionID) == "" {
		return middleware.SessionActionResult{}, fmt.Errorf("session %s has no remote session id to fork", meta.ID)
	}
	forker, ok := m.router.(middleware.AgentSessionForker)
	if !ok {
		return unsupportedForkResult(meta, "router does not expose session fork"), nil
	}
	child, err := forker.ForkAgentSession(ctx, meta.AgentID, middleware.SessionForkRequest{
		RemoteSessionID: meta.AgentSessionID,
		WorkspacePath:   firstNonEmpty(req.WorkspacePath, meta.WorkspacePath),
	})
	if err != nil {
		return unsupportedForkResult(meta, err.Error()), nil
	}
	childMeta := meta
	childMeta.ID = uuid.New().String()
	childMeta.AgentSessionID = child.RemoteSessionID
	childMeta.CreatedAt = time.Now().UTC()
	childMeta.Alias = ""
	childMeta.RemoteTitle = child.Title
	childMeta.RemoteStatus = child.Status
	childMeta.LastSyncedAt = time.Now().UTC()
	childMeta.RemoteUpdatedAt = time.Time{}
	childMeta.PendingHandoff = nil
	childMeta.LastHandoff = nil
	if child.UpdatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, child.UpdatedAt); err == nil {
			childMeta.RemoteUpdatedAt = parsed
		}
	}
	if err := m.saveSessionMeta(childMeta); err != nil {
		return middleware.SessionActionResult{}, err
	}
	if err := m.updateChannelState(req.ChannelID, childMeta.ID); err != nil {
		return middleware.SessionActionResult{}, err
	}
	if err := m.indexSessionWorkspace(childMeta); err != nil {
		return middleware.SessionActionResult{}, err
	}
	return middleware.SessionActionResult{
		Action:          "fork",
		Message:         fmt.Sprintf("Forked session: %s", childMeta.ID),
		ActiveSessionID: childMeta.ID,
		Session:         m.toSessionEntry(childMeta, true),
		Fork: &middleware.SessionForkResult{
			ParentRemoteSessionID: meta.AgentSessionID,
			Child:                 &child,
		},
	}, nil
}

func (m *Manager) handleSessionReconcileTyped(ctx context.Context, _ middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	reconciler, ok := m.router.(middleware.AgentClientReconciler)
	if !ok {
		return middleware.SessionActionResult{
			Action:      "reconcile",
			Message:     "Agent client reconcile is not available.",
			Unsupported: true,
		}, nil
	}
	active, err := m.activeAgentClientRefs()
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	result, err := reconciler.ReconcileAgentClients(ctx, active)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	return middleware.SessionActionResult{
		Action:    "reconcile",
		Message:   fmt.Sprintf("Reconciled agent clients: reaped=%d retained=%d", len(result.Reaped), len(result.Retained)),
		Reconcile: &result,
	}, nil
}

func (m *Manager) resolveActionAgentID(req middleware.SessionActionRequest) (string, error) {
	if target := strings.TrimSpace(req.Target); target != "" {
		return target, nil
	}
	state, err := m.getChannelState(req.ChannelID)
	if err != nil {
		return "", err
	}
	meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
	if err != nil {
		return "", err
	}
	if found && strings.TrimSpace(meta.AgentID) != "" {
		return meta.AgentID, nil
	}
	if strings.TrimSpace(m.defaultAgent) != "" {
		return m.defaultAgent, nil
	}
	return "", fmt.Errorf("agent id is required")
}

func (m *Manager) resolveActionSession(req middleware.SessionActionRequest) (SessionMeta, error) {
	targetID := strings.TrimSpace(req.Target)
	state, err := m.getChannelState(req.ChannelID)
	if err != nil {
		return SessionMeta{}, err
	}
	if targetID == "" {
		targetID = state.ActiveSessionID
	}
	metas, err := m.loadSessionMetas(state.History)
	if err != nil {
		return SessionMeta{}, err
	}
	if resolved := resolveSessionTarget(targetID, state, metas); resolved != "" {
		targetID = resolved
	}
	meta, found, err := m.loadSessionMeta(targetID)
	if err != nil {
		return SessionMeta{}, err
	}
	if !found {
		return SessionMeta{}, fmt.Errorf("session %s not found", targetID)
	}
	return meta, nil
}

func (m *Manager) activeAgentClientRefs() ([]middleware.AgentClientRef, error) {
	keys, err := m.storage.List("session.meta.")
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := []middleware.AgentClientRef{}
	for _, key := range keys {
		id := strings.TrimPrefix(key, "session.meta.")
		meta, found, err := m.loadSessionMeta(id)
		if err != nil || !found {
			continue
		}
		agentID := strings.TrimSpace(meta.AgentID)
		if agentID == "" {
			continue
		}
		workspacePath := strings.TrimSpace(meta.WorkspacePath)
		dedupePath := workspacePath
		if dedupePath != "" {
			dedupePath = cleanSessionWorkspacePath(dedupePath)
		}
		dedupe := agentID + "\x00" + dedupePath
		if _, ok := seen[dedupe]; ok {
			continue
		}
		seen[dedupe] = struct{}{}
		out = append(out, middleware.AgentClientRef{AgentID: agentID, WorkspacePath: workspacePath})
	}
	return out, nil
}

func unsupportedForkResult(meta SessionMeta, reason string) middleware.SessionActionResult {
	return middleware.SessionActionResult{
		Action:      "fork",
		Message:     "Session fork is unsupported by this provider.",
		Unsupported: true,
		Fork: &middleware.SessionForkResult{
			ParentRemoteSessionID: meta.AgentSessionID,
			Unsupported:           true,
			Reason:                reason,
		},
	}
}
