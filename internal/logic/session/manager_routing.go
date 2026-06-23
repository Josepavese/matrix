package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/sessionqueue"
	"github.com/Josepavese/matrix/internal/middleware"
)

// Route handles an incoming message from a channel. It uses the SSOT Vault to
// lookup the correct SessionID and delegates to the AgentRouter.
func (m *Manager) Route(ctx context.Context, channelID string, agentID string, input string, notifier middleware.ThoughtNotifier) (string, error) {
	return m.RouteConversation(ctx, middleware.ConversationRequest{
		ChannelID: channelID,
		AgentID:   agentID,
		Input:     input,
		Notifier:  notifier,
	})
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
	if header, ok := notifier.(interface{ SetLogicalSession(string, string) }); ok {
		header.SetLogicalSession(sessionID, meta.WorkspaceID)
	}
	if strings.TrimSpace(input) != "" {
		m.recordWorkspaceTurn(meta, "user", input)
	}
	message := input
	if handoffPrompt := renderHandoffPrompt(meta.PendingHandoff); handoffPrompt != "" {
		message = handoffPrompt + "\n\nUser request:\n" + input
	}
	responseTxt, newAgentSessionID, toolCalls, metadata, routeErr := m.router.Route(ctx, middleware.RouteRequest{
		AgentID:               effectiveAgentID,
		LogicalSessionID:      sessionID,
		AgentSessionID:        meta.AgentSessionID,
		WorkspacePath:         meta.WorkspacePath,
		Message:               message,
		ContentBlocks:         req.ContentBlocks,
		SidecarCapsules:       req.SidecarCapsules,
		AdditionalDirectories: req.AdditionalDirectories,
		AgentLaunchArgs:       req.AgentLaunchArgs,
		ThoughtNotifier:       notifier,
	})
	m.applyPendingHandoff(&meta, channelID, log, routeErr)

	responseTxt = m.applyToolCalls(responseTxt, toolCalls)
	meta.AgentID = effectiveAgentID
	queue.Submit(seq, sessionqueue.RouteResult{
		LogicalSessionID: sessionID,
		Content:          responseTxt,
		AgentSessionID:   newAgentSessionID,
		Metadata:         metadata,
		Err:              routeErr,
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

func (m *Manager) applyPendingHandoff(meta *SessionMeta, channelID string, log *slog.Logger, routeErr error) {
	if routeErr != nil || meta.PendingHandoff == nil {
		return
	}
	meta.LastHandoff = meta.PendingHandoff
	m.recordWorkspaceEvent(*meta, "handoff.applied", channelID, "Applied specialist handoff", "specialist-handoff", handoffMetadata(*meta))
	meta.PendingHandoff = nil
	if err := m.saveSessionMeta(*meta); err != nil {
		log.Warn("failed to clear pending handoff", "error", err, "logical_session", meta.ID)
	}
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
	if routeErr != nil && strings.TrimSpace(newAgentSessionID) == "" {
		return
	}

	log := slog.With("component", "session_manager", "logical_session", meta.ID, "agent", meta.AgentID)
	now := time.Now().UTC()
	meta.LastSyncedAt = now
	changed := applyAgentSessionMirrorID(&meta, newAgentSessionID)
	changed = m.applyAgentSessionProtocolKind(&meta) || changed
	changed = applyAgentSessionMirrorStatus(&meta) || changed
	changed = applyAgentSessionMetadata(&meta, metadata, now) || changed
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

func applyAgentSessionMirrorID(meta *SessionMeta, newAgentSessionID string) bool {
	if strings.TrimSpace(newAgentSessionID) == "" || newAgentSessionID == meta.AgentSessionID {
		return false
	}
	meta.AgentSessionID = newAgentSessionID
	return true
}

func (m *Manager) applyAgentSessionProtocolKind(meta *SessionMeta) bool {
	endpoint, err := m.resolveEndpoint(meta.AgentID)
	if err != nil {
		return false
	}
	kind := string(endpoint.Kind)
	if kind == "" || kind == meta.ProtocolKind {
		return false
	}
	meta.ProtocolKind = kind
	return true
}

func applyAgentSessionMirrorStatus(meta *SessionMeta) bool {
	if meta.MirrorStatus == "mirrored" {
		return false
	}
	meta.MirrorStatus = "mirrored"
	return true
}

func applyAgentSessionMetadata(meta *SessionMeta, metadata middleware.ConversationMetadata, fallbackUpdatedAt time.Time) bool {
	changed := setStringIfNew(&meta.RemoteTitle, metadata.Title)
	changed = setStringIfNew(&meta.RemoteStatus, metadata.Status) || changed
	changed = mergeRemoteMeta(meta, metadata.Meta) || changed
	changed = applyRemoteUpdatedAt(meta, metadata.UpdatedAt, fallbackUpdatedAt) || changed
	return changed
}

func setStringIfNew(target *string, value string) bool {
	if value == "" || value == *target {
		return false
	}
	*target = value
	return true
}

func mergeRemoteMeta(meta *SessionMeta, values map[string]interface{}) bool {
	if len(values) == 0 {
		return false
	}
	if meta.RemoteMeta == nil {
		meta.RemoteMeta = make(map[string]interface{}, len(values))
	}
	changed := false
	for k, v := range values {
		if existing, ok := meta.RemoteMeta[k]; !ok || !reflect.DeepEqual(existing, v) {
			meta.RemoteMeta[k] = v
			changed = true
		}
	}
	return changed
}

func applyRemoteUpdatedAt(meta *SessionMeta, updatedAt string, fallback time.Time) bool {
	if parsed, ok := parseRemoteUpdatedAt(updatedAt); ok && !parsed.Equal(meta.RemoteUpdatedAt) {
		meta.RemoteUpdatedAt = parsed
		return true
	}
	if meta.RemoteUpdatedAt.IsZero() {
		meta.RemoteUpdatedAt = fallback
		return true
	}
	return false
}

func parseRemoteUpdatedAt(value string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	return parsed, err == nil
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
