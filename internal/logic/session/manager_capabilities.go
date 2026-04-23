package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
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
		if isAgentNotFound(err) {
			return agentNotFoundCapabilitiesResult(agentID), nil
		}
		return middleware.SessionActionResult{}, err
	}
	return middleware.SessionActionResult{
		Action:       "capabilities",
		Message:      "Provider capabilities loaded.",
		Capabilities: &report,
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

func isAgentNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, middleware.ErrAgentNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found in registry") ||
		strings.Contains(msg, "configuration not found in registry") ||
		strings.Contains(msg, "agent not found")
}

func agentNotFoundCapabilitiesResult(agentID string) middleware.SessionActionResult {
	return middleware.SessionActionResult{
		Action:      "capabilities",
		Message:     "Agent id is not registered.",
		Unsupported: true,
		Error: &middleware.SessionActionError{
			Code:    "agent_not_found",
			Message: "agent id is not registered",
			Target:  agentID,
		},
		Capabilities: &middleware.ProviderCapabilityReport{
			AgentID: agentID,
		},
	}
}
