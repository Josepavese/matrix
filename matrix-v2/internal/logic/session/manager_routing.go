package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jose/matrix-v2/internal/middleware"
)

// Route handles an incoming message from a channel. It uses the SSOT Vault to
// lookup the correct SessionID and delegates to the AgentRouter.
// It also intercepts "/session" commands.
func (m *Manager) Route(ctx context.Context, channelID string, agentID string, input string, notifier middleware.ThoughtNotifier) (string, error) {
	if !m.wizard.IsConfigured() {
		return m.wizard.Process(channelID, input)
	}
	if handled, response, err := m.tryHandleCommand(ctx, channelID, input); handled {
		return response, err
	}
	return m.routeAgentTurn(ctx, channelID, agentID, input, notifier)
}

func (m *Manager) tryHandleCommand(ctx context.Context, channelID, input string) (bool, string, error) {
	trimmed := strings.TrimSpace(input)
	switch {
	case strings.HasPrefix(trimmed, "/session"):
		response, err := m.handleSessionCommand(channelID, input)
		return true, response, err
	case strings.HasPrefix(trimmed, "/action"):
		response, err := m.handleActionCommand(ctx, channelID, input)
		return true, response, err
	case strings.HasPrefix(trimmed, "/help"):
		response, err := m.handleHelpCommand(channelID)
		return true, response, err
	case strings.HasPrefix(trimmed, "/wizard"):
		response, err := m.handleWizardCommand(channelID)
		return true, response, err
	default:
		return false, "", nil
	}
}

func (m *Manager) routeAgentTurn(ctx context.Context, channelID, agentID, input string, notifier middleware.ThoughtNotifier) (string, error) {
	log := slog.With("component", "session_manager", "channel", channelID)
	sessionID, err := m.GetOrCreateSession(channelID, agentID)
	if err != nil {
		return "", fmt.Errorf("failed to route session: %w", err)
	}
	log.Info("routing channel input", "event", "route_started", "logical_session", sessionID, "requested_agent", agentID, "input_len", len(input))

	queue := m.getOrCreateQueue(channelID)
	seq := queue.NextSeq()

	meta, effectiveAgentID := m.resolveRouteMeta(sessionID, agentID)
	responseTxt, newAgentSessionID, toolCalls, routeErr := m.router.Route(ctx, middleware.RouteRequest{
		AgentID:          effectiveAgentID,
		LogicalSessionID: sessionID,
		AgentSessionID:   meta.AgentSessionID,
		Message:          input,
		ThoughtNotifier:  notifier,
	})

	responseTxt = m.applyToolCalls(responseTxt, toolCalls)
	meta.AgentID = effectiveAgentID
	queue.Submit(seq, RouteResult{
		Content:        responseTxt,
		AgentSessionID: newAgentSessionID,
		Err:            routeErr,
	})
	if routeErr == nil {
		log.Info("completed routed turn", "event", "route_completed", "logical_session", sessionID, "agent", effectiveAgentID, "response_len", len(responseTxt), "tool_calls", len(toolCalls))
	}
	return responseTxt, routeErr
}

func (m *Manager) resolveRouteMeta(sessionID, requestedAgentID string) (SessionMeta, string) {
	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil || !found {
		return SessionMeta{ID: sessionID, AgentID: requestedAgentID}, requestedAgentID
	}
	return meta, meta.AgentID
}

func (m *Manager) applyToolCalls(response string, toolCalls []middleware.ToolCall) string {
	if len(toolCalls) == 0 || m.systemTools == nil {
		return response
	}
	for _, tc := range toolCalls {
		response += "\n" + m.systemTools.ExecuteTool(tc)
	}
	return response
}

func (m *Manager) persistAgentSession(meta SessionMeta, newAgentSessionID string, routeErr error) {
	if routeErr != nil || newAgentSessionID == "" || newAgentSessionID == meta.AgentSessionID {
		return
	}

	log := slog.With("component", "session_manager", "logical_session", meta.ID, "agent", meta.AgentID)
	meta.AgentSessionID = newAgentSessionID
	metaData, err := json.Marshal(meta)
	if err != nil {
		log.Warn("failed to encode updated agent session mapping", "event", "agent_session_encode_failed", "error", err)
		return
	}
	if err := m.storage.Set(getSessionKey(meta.ID), metaData); err != nil {
		log.Warn("failed to persist updated agent session mapping", "event", "agent_session_update_failed", "error", err)
		return
	}
	log.Info("updated stored acp session mapping", "event", "agent_session_updated", "agent_session", newAgentSessionID)
}
