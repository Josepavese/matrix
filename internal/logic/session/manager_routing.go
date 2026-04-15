package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

// Route handles an incoming message from a channel. It uses the SSOT Vault to
// lookup the correct SessionID and delegates to the AgentRouter.
// It also intercepts "/session" commands.
func (m *Manager) Route(ctx context.Context, channelID string, agentID string, input string, notifier middleware.ThoughtNotifier) (string, error) {
	return m.RouteConversation(ctx, middleware.ConversationRequest{
		ChannelID: channelID,
		AgentID:   agentID,
		Input:     input,
		Notifier:  notifier,
	})
}

func (m *Manager) tryHandleCommand(ctx context.Context, channelID, input string) (bool, string, error) {
	trimmed := strings.TrimSpace(input)
	switch {
	case strings.HasPrefix(trimmed, "/workspaces"):
		response, err := m.HandleWorkspaceAction(ctx, channelID, "list", "")
		return true, response, err
	case strings.HasPrefix(trimmed, "/now"):
		response, err := m.HandleWorkspaceRead(ctx, channelID, "state", "", 0)
		return true, response, err
	case strings.HasPrefix(trimmed, "/timeline"):
		args := strings.TrimSpace(strings.TrimPrefix(trimmed, "/timeline"))
		response, err := m.HandleWorkspaceRead(ctx, channelID, "timeline", args, 10)
		return true, response, err
	case strings.HasPrefix(trimmed, "/decisions"):
		args := strings.TrimSpace(strings.TrimPrefix(trimmed, "/decisions"))
		response, err := m.HandleWorkspaceRead(ctx, channelID, "decisions", args, 10)
		return true, response, err
	case strings.HasPrefix(trimmed, "/why"):
		response, err := m.HandleWorkspaceRead(ctx, channelID, "decisions", "", 1)
		return true, response, err
	case strings.HasPrefix(trimmed, "/memory"):
		response, err := m.HandleWorkspaceRead(ctx, channelID, "memory", "", 12)
		return true, response, err
	case strings.HasPrefix(trimmed, "/snapshots"):
		response, err := m.HandleWorkspaceRead(ctx, channelID, "snapshots", "", 10)
		return true, response, err
	case strings.HasPrefix(trimmed, "/snapshot"):
		args := strings.TrimSpace(strings.TrimPrefix(trimmed, "/snapshot"))
		response, err := m.HandleWorkspaceAction(ctx, channelID, "snapshot", args)
		return true, response, err
	case strings.HasPrefix(trimmed, "/status"):
		response, err := m.handleStatusCommand(channelID)
		return true, response, err
	case strings.HasPrefix(trimmed, "/review"):
		args := strings.TrimSpace(strings.TrimPrefix(trimmed, "/review"))
		response, err := m.handleModeAction(ctx, channelID, modeReview, args)
		return true, response, err
	case strings.HasPrefix(trimmed, "/continue"):
		response, err := m.HandleIntent(ctx, channelID, "continue", "")
		return true, response, err
	case strings.HasPrefix(trimmed, "/resume"):
		args := strings.TrimSpace(strings.TrimPrefix(trimmed, "/resume"))
		response, err := m.HandleIntent(ctx, channelID, "resume", args)
		return true, response, err
	case strings.HasPrefix(trimmed, "/explain"):
		args := strings.TrimSpace(strings.TrimPrefix(trimmed, "/explain"))
		response, err := m.handleModeAction(ctx, channelID, modeExplain, args)
		return true, response, err
	case strings.HasPrefix(trimmed, "/triage"):
		args := strings.TrimSpace(strings.TrimPrefix(trimmed, "/triage"))
		response, err := m.handleModeAction(ctx, channelID, modeTriage, args)
		return true, response, err
	case strings.HasPrefix(trimmed, "/handoff"):
		args := strings.TrimSpace(strings.TrimPrefix(trimmed, "/handoff"))
		response, err := m.HandleIntent(ctx, channelID, "handoff", args)
		return true, response, err
	case strings.HasPrefix(trimmed, "/session"):
		response, err := m.handleSessionCommand(ctx, channelID, input)
		return true, response, err
	case strings.HasPrefix(trimmed, "/workspace"):
		response, err := m.handleWorkspaceCommand(ctx, channelID, input)
		return true, response, err
	case strings.HasPrefix(trimmed, "/use"):
		args := strings.TrimSpace(strings.TrimPrefix(trimmed, "/use"))
		response, err := m.HandleWorkspaceAction(ctx, channelID, "switch", args)
		return true, response, err
	case strings.HasPrefix(trimmed, "/cancel"), strings.HasPrefix(trimmed, "/stop"):
		args := strings.TrimSpace(strings.TrimPrefix(trimmed, strings.Fields(trimmed)[0]))
		response, err := m.handleSessionCancel(ctx, channelID, m.wizard.GetLanguage(channelID), args)
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
	return m.routeResolvedSession(ctx, middleware.ConversationRequest{
		ChannelID: channelID,
		AgentID:   agentID,
		Input:     input,
		Notifier:  notifier,
	}, "", agentID)
}

func (m *Manager) routeResolvedSession(ctx context.Context, req middleware.ConversationRequest, preResolvedSessionID string, fallbackAgentID string) (string, error) {
	channelID := req.ChannelID
	agentID := req.AgentID
	input := req.Input
	notifier := req.Notifier
	log := slog.With("component", "session_manager", "channel", channelID)
	if strings.TrimSpace(agentID) == "" {
		agentID = fallbackAgentID
	}
	if strings.TrimSpace(agentID) == "" {
		agentID = m.defaultAgent
	}
	sessionID := preResolvedSessionID
	if strings.TrimSpace(sessionID) == "" {
		var err error
		sessionID, err = m.GetOrCreateSession(channelID, agentID)
		if err != nil {
			return "", fmt.Errorf("failed to route session: %w", err)
		}
	}
	log.Info("routing channel input", "event", "route_started", "logical_session", sessionID, "requested_agent", agentID, "input_len", len(input))

	queue := m.getOrCreateQueue(channelID)
	seq := queue.NextSeq()

	meta, effectiveAgentID := m.resolveRouteMeta(sessionID, agentID)
	if meta.Mode == "" {
		meta.Mode = modeImplementation
	}
	if strings.TrimSpace(input) != "" {
		m.recordWorkspaceTurn(meta, "user", input)
	}
	message := input
	if handoffPrompt := renderHandoffPrompt(meta.PendingHandoff); handoffPrompt != "" {
		message = handoffPrompt + "\n\nUser request:\n" + input
	}
	responseTxt, newAgentSessionID, toolCalls, metadata, routeErr := m.router.Route(ctx, middleware.RouteRequest{
		AgentID:          effectiveAgentID,
		LogicalSessionID: sessionID,
		AgentSessionID:   meta.AgentSessionID,
		Message:          message,
		ThoughtNotifier:  notifier,
	})
	if routeErr == nil && meta.PendingHandoff != nil {
		meta.LastHandoff = meta.PendingHandoff
		m.recordWorkspaceEvent(meta, "handoff.applied", channelID, "Applied specialist handoff", "specialist-handoff", handoffMetadata(meta))
		meta.PendingHandoff = nil
		if err := m.saveSessionMeta(meta); err != nil {
			log.Warn("failed to clear pending handoff", "error", err, "logical_session", sessionID)
		}
	}

	responseTxt = m.applyToolCalls(responseTxt, toolCalls)
	meta.AgentID = effectiveAgentID
	queue.Submit(seq, RouteResult{
		Content:        responseTxt,
		AgentSessionID: newAgentSessionID,
		Metadata:       metadata,
		Err:            routeErr,
	})
	if routeErr == nil {
		if strings.TrimSpace(responseTxt) != "" {
			m.recordWorkspaceTurn(meta, "assistant", responseTxt)
		}
		log.Info("completed routed turn", "event", "route_completed", "logical_session", sessionID, "agent", effectiveAgentID, "response_len", len(responseTxt), "tool_calls", len(toolCalls))
	}
	return responseTxt, routeErr
}

func (m *Manager) resolveRouteMeta(sessionID, requestedAgentID string) (SessionMeta, string) {
	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil || !found {
		return SessionMeta{ID: sessionID, AgentID: requestedAgentID, MirrorStatus: "pending"}, requestedAgentID
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

func (m *Manager) persistAgentSession(meta SessionMeta, newAgentSessionID string, metadata middleware.ConversationMetadata, routeErr error) {
	if routeErr != nil {
		return
	}

	log := slog.With("component", "session_manager", "logical_session", meta.ID, "agent", meta.AgentID)
	changed := false
	if newAgentSessionID != "" && newAgentSessionID != meta.AgentSessionID {
		meta.AgentSessionID = newAgentSessionID
		changed = true
	}
	if endpoint, err := m.resolveEndpoint(meta.AgentID); err == nil {
		if kind := string(endpoint.Kind); kind != "" && kind != meta.ProtocolKind {
			meta.ProtocolKind = kind
			changed = true
		}
	}
	now := time.Now().UTC()
	if meta.MirrorStatus != "mirrored" {
		meta.MirrorStatus = "mirrored"
		changed = true
	}
	meta.LastSyncedAt = now
	if metadata.Title != "" && metadata.Title != meta.RemoteTitle {
		meta.RemoteTitle = metadata.Title
		changed = true
	}
	if metadata.Status != "" && metadata.Status != meta.RemoteStatus {
		meta.RemoteStatus = metadata.Status
		changed = true
	}
	if len(metadata.Meta) > 0 {
		if meta.RemoteMeta == nil {
			meta.RemoteMeta = make(map[string]interface{}, len(metadata.Meta))
		}
		for k, v := range metadata.Meta {
			if existing, ok := meta.RemoteMeta[k]; !ok || !reflect.DeepEqual(existing, v) {
				meta.RemoteMeta[k] = v
				changed = true
			}
		}
	}
	if metadata.UpdatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, metadata.UpdatedAt); err == nil {
			if !parsed.Equal(meta.RemoteUpdatedAt) {
				meta.RemoteUpdatedAt = parsed
				changed = true
			}
		}
	}
	if meta.RemoteUpdatedAt.IsZero() {
		meta.RemoteUpdatedAt = now
		changed = true
	}
	if !changed {
		return
	}
	if err := m.indexSessionWorkspace(meta); err != nil {
		log.Warn("failed to update workspace session index", "error", err, "workspace_id", meta.WorkspaceID)
	}
	if err := m.saveSessionMeta(meta); err != nil {
		log.Warn("failed to persist updated agent session mapping", "event", "agent_session_update_failed", "error", err)
		return
	}
	log.Info("updated stored acp session mapping", "event", "agent_session_updated", "agent_session", newAgentSessionID)
}

func (m *Manager) saveSessionMeta(meta SessionMeta) error {
	metaData, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal session meta: %w", err)
	}
	return m.storage.Set(getSessionKey(meta.ID), metaData)
}

func (m *Manager) resolveEndpoint(agentID string) (middleware.ProtocolEndpoint, error) {
	if m.resolver == nil {
		return middleware.ProtocolEndpoint{}, fmt.Errorf("endpoint resolver not configured")
	}
	return m.resolver.GetAgentEndpoint(agentID)
}
