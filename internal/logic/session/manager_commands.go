package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jose/matrix-v2/internal/middleware"
)

func (m *Manager) handleSessionStatus(channelID, lang string) (string, error) {
	result, err := m.handleSessionStatusTyped(channelID, lang, "")
	if err != nil {
		return "", err
	}
	return m.renderSessionAction(result, lang), nil
}

func (m *Manager) handleSessionStatusTyped(channelID, lang, workspaceID string) (middleware.SessionActionResult, error) {
	if state, err := m.getChannelState(channelID); err == nil && strings.TrimSpace(workspaceID) == "" && strings.TrimSpace(state.ActiveSessionID) != "" {
		meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
		if err != nil {
			return middleware.SessionActionResult{}, err
		}
		if found {
			return middleware.SessionActionResult{
				Action:          "status",
				ActiveSessionID: meta.ID,
				Session:         m.toSessionEntry(meta, true),
			}, nil
		}
	}
	sessionID, _, err := m.getOrCreateSessionForWorkspace(channelID, "", workspaceID, "")
	if err != nil {
		return middleware.SessionActionResult{}, err
	}

	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if !found {
		return middleware.SessionActionResult{Action: "status", Message: m.wizard.GetString(lang, "session_not_found_db")}, nil
	}
	return middleware.SessionActionResult{
		Action:          "status",
		ActiveSessionID: meta.ID,
		Session:         m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) handleSessionName(channelID, lang, alias string) (string, error) {
	result, err := m.handleSessionNameTyped(channelID, lang, alias)
	if err != nil {
		return "", err
	}
	return m.renderSessionAction(result, lang), nil
}

func (m *Manager) handleSessionNameTyped(channelID, lang, alias string) (middleware.SessionActionResult, error) {
	if alias == "" {
		return middleware.SessionActionResult{Action: "name", Message: m.wizard.GetString(lang, "session_name_usage")}, nil
	}

	sessionID, err := m.GetOrCreateSession(channelID, m.defaultAgent)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}

	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if !found {
		return middleware.SessionActionResult{Action: "name", Message: m.wizard.GetString(lang, "session_not_found_db")}, nil
	}

	meta.Alias = alias
	newData, err := json.Marshal(meta)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if err := m.storage.Set(getSessionKey(sessionID), newData); err != nil {
		return middleware.SessionActionResult{}, err
	}
	return middleware.SessionActionResult{
		Action:          "name",
		Message:         fmt.Sprintf(m.wizard.GetString(lang, "session_alias_set"), alias),
		ActiveSessionID: meta.ID,
		Session:         m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) handleSessionNew(channelID, lang string, parts []string) (string, error) {
	agentID := m.defaultAgent
	if len(parts) >= 3 {
		agentID = parts[2]
	}
	result, err := m.handleSessionNewTyped(channelID, lang, agentID, "", "")
	if err != nil {
		return "", err
	}
	return m.renderSessionAction(result, lang), nil
}

func (m *Manager) handleSessionNewTyped(channelID, lang, agentID, workspaceID, workspacePath string) (middleware.SessionActionResult, error) {
	resolvedAgentID := m.defaultAgent
	if strings.TrimSpace(agentID) != "" {
		resolvedAgentID = strings.TrimSpace(agentID)
	}

	sessionID, err := m.forceNewSessionWithWorkspace(channelID, resolvedAgentID, workspaceID, workspacePath)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	meta, _, _ := m.loadSessionMeta(sessionID)
	return middleware.SessionActionResult{
		Action:          "new",
		Message:         fmt.Sprintf(m.wizard.GetString(lang, "session_new_started"), resolvedAgentID, sessionID),
		ActiveSessionID: sessionID,
		Session:         m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) handleSessionList(ctx context.Context, channelID, lang string) (string, error) {
	result, err := m.handleSessionListTyped(ctx, channelID, lang, "")
	if err != nil {
		return "", err
	}
	return m.renderSessionAction(result, lang), nil
}

func (m *Manager) handleSessionListTyped(ctx context.Context, channelID, lang, workspaceID string) (middleware.SessionActionResult, error) {
	state, err := m.getChannelState(channelID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}

	metas, err := m.loadSessionMetas(state.History)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	local := make([]middleware.SessionEntry, 0, len(metas))
	for _, meta := range metas {
		if workspaceID != "" && meta.WorkspaceID != workspaceID {
			continue
		}
		local = append(local, *m.toSessionEntry(meta, meta.ID == state.ActiveSessionID))
	}

	remote, _, remoteErr := m.listRemoteSessionsForChannel(ctx, channelID)
	result := middleware.SessionActionResult{
		Action:          "list",
		ActiveSessionID: state.ActiveSessionID,
		Sessions:        local,
		RemoteSessions:  remote,
	}
	if len(local) == 0 && len(remote) == 0 {
		if remoteErr != nil {
			result.Message = fmt.Sprintf("%s\nRemote discovery unavailable: %v", m.wizard.GetString(lang, "session_history_empty"), remoteErr)
		} else {
			result.Message = m.wizard.GetString(lang, "session_history_empty")
		}
	}
	return result, nil
}

func (m *Manager) handleSessionSwitch(ctx context.Context, channelID, lang, args string) (string, error) {
	result, err := m.handleSessionSwitchTyped(ctx, channelID, lang, args)
	if err != nil {
		return "", err
	}
	return m.renderSessionAction(result, lang), nil
}

func (m *Manager) handleSessionSwitchTyped(ctx context.Context, channelID, lang, args string) (middleware.SessionActionResult, error) {
	state, err := m.getChannelState(channelID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if len(state.History) == 0 {
		return middleware.SessionActionResult{Action: "switch", Message: "No session history to switch to."}, nil
	}

	if args == "" {
		return m.switchToPreviousSessionTyped(channelID, lang, state)
	}

	metas, err := m.loadSessionMetas(state.History)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}

	targetID := resolveSessionTarget(args, state, metas)
	if targetID == "" {
		if imported, handled, err := m.trySwitchToRemoteSession(ctx, channelID, args); handled {
			if err != nil {
				return middleware.SessionActionResult{}, err
			}
			return imported, nil
		}
		return m.createRequestedAgentSessionTyped(channelID, lang, args)
	}
	if err := m.attachChannelWithEvent(channelID, targetID, "session.switched", "Switched active session", "session-switch", nil); err != nil {
		return middleware.SessionActionResult{}, err
	}
	meta, _, _ := m.loadSessionMeta(targetID)
	return middleware.SessionActionResult{
		Action:          "switch",
		Message:         fmt.Sprintf(m.wizard.GetString(lang, "session_switched"), targetID),
		ActiveSessionID: targetID,
		Session:         m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) handleSessionDelete(ctx context.Context, channelID, lang, args string) (string, error) {
	result, err := m.handleSessionDeleteTyped(ctx, channelID, lang, args)
	if err != nil {
		return "", err
	}
	return m.renderSessionAction(result, lang), nil
}

func (m *Manager) handleSessionDeleteTyped(ctx context.Context, channelID, lang, args string) (middleware.SessionActionResult, error) {
	targetID := strings.TrimSpace(args)
	if targetID == "" {
		state, err := m.getChannelState(channelID)
		if err != nil {
			return middleware.SessionActionResult{}, err
		}
		targetID = state.ActiveSessionID
	}
	if targetID == "" {
		return middleware.SessionActionResult{Action: "delete", Message: m.wizard.GetString(lang, "session_history_empty")}, nil
	}

	state, err := m.getChannelState(channelID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	metas, err := m.loadSessionMetas(state.History)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	resolved := resolveSessionTarget(targetID, state, metas)
	if resolved != "" {
		targetID = resolved
	}

	meta, found, err := m.loadSessionMeta(targetID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if !found {
		deleted, handled, err := m.tryDeleteRemoteSession(ctx, channelID, targetID)
		if handled {
			if err != nil {
				return middleware.SessionActionResult{}, err
			}
			return deleted, nil
		}
		return middleware.SessionActionResult{}, fmt.Errorf("session %s not found", targetID)
	}

	if meta.AgentSessionID != "" {
		if err := m.deleteRemoteSession(ctx, meta); err != nil {
			return middleware.SessionActionResult{}, err
		}
	}
	if err := m.removeSessionMirror(channelID, meta.ID); err != nil {
		return middleware.SessionActionResult{}, err
	}
	m.recordWorkspaceEvent(meta, "session.deleted", channelID, "Deleted workspace session", "session-delete", nil)
	return middleware.SessionActionResult{
		Action:  "delete",
		Message: fmt.Sprintf("Deleted session: %s", meta.ID),
		Session: m.toSessionEntry(meta, false),
	}, nil
}

func (m *Manager) handleSessionCancel(ctx context.Context, channelID, lang, args string) (string, error) {
	result, err := m.handleSessionCancelTyped(ctx, channelID, lang, args)
	if err != nil {
		return "", err
	}
	return m.renderSessionAction(result, lang), nil
}

func (m *Manager) handleSessionCancelTyped(ctx context.Context, channelID, lang, args string) (middleware.SessionActionResult, error) {
	targetID := strings.TrimSpace(args)
	if targetID == "" {
		state, err := m.getChannelState(channelID)
		if err != nil {
			return middleware.SessionActionResult{}, err
		}
		targetID = state.ActiveSessionID
	}
	if targetID == "" {
		return middleware.SessionActionResult{Action: "cancel", Message: m.wizard.GetString(lang, "session_history_empty")}, nil
	}

	state, err := m.getChannelState(channelID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	metas, err := m.loadSessionMetas(state.History)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if resolved := resolveSessionTarget(targetID, state, metas); resolved != "" {
		targetID = resolved
	}

	meta, found, err := m.loadSessionMeta(targetID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if !found {
		canceled, handled, err := m.tryCancelRemoteSession(ctx, channelID, targetID)
		if handled {
			if err != nil {
				return middleware.SessionActionResult{}, err
			}
			return canceled, nil
		}
		return middleware.SessionActionResult{}, fmt.Errorf("session %s not found", targetID)
	}
	if strings.TrimSpace(meta.AgentSessionID) == "" {
		return middleware.SessionActionResult{}, fmt.Errorf("session %s has no remote session id to cancel", meta.ID)
	}
	if err := m.cancelRemoteSession(ctx, meta); err != nil {
		return middleware.SessionActionResult{}, err
	}
	meta.RemoteStatus = "canceled"
	meta.LastSyncedAt = time.Now().UTC()
	if err := m.saveSessionMeta(meta); err != nil {
		return middleware.SessionActionResult{}, err
	}
	m.recordWorkspaceEvent(meta, "session.canceled", channelID, "Canceled workspace session", "session-cancel", nil)
	return middleware.SessionActionResult{
		Action:          "cancel",
		Message:         fmt.Sprintf("Canceled session: %s", meta.ID),
		ActiveSessionID: meta.ID,
		Session:         m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) switchToPreviousSessionTyped(channelID, lang string, state ChannelState) (middleware.SessionActionResult, error) {
	if len(state.History) <= 1 {
		return middleware.SessionActionResult{Action: "switch", Message: m.wizard.GetString(lang, "session_history_switch_no_prev")}, nil
	}
	if err := m.attachChannelWithEvent(channelID, state.History[1], "session.switched", "Switched to previous session", "session-switch-prev", nil); err != nil {
		return middleware.SessionActionResult{}, err
	}
	meta, _, _ := m.loadSessionMeta(state.History[1])
	return middleware.SessionActionResult{
		Action:          "switch",
		Message:         m.wizard.GetString(lang, "session_history_switch_prev"),
		ActiveSessionID: state.History[1],
		Session:         m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) createRequestedAgentSessionTyped(channelID, lang, args string) (middleware.SessionActionResult, error) {
	requestedAgentID := strings.Fields(args)[0]
	sessionID, err := m.forceNewSessionWithWorkspace(channelID, requestedAgentID, "", "")
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	meta, _, _ := m.loadSessionMeta(sessionID)
	return middleware.SessionActionResult{
		Action:          "new",
		Message:         fmt.Sprintf(m.wizard.GetString(lang, "session_switch_resolve_fail_new"), requestedAgentID, sessionID),
		ActiveSessionID: sessionID,
		Session:         m.toSessionEntry(meta, true),
	}, nil
}

func (m *Manager) loadSessionMeta(sessionID string) (SessionMeta, bool, error) {
	data, err := m.storage.Get(getSessionKey(sessionID))
	if err != nil {
		return SessionMeta{}, false, err
	}
	if len(data) == 0 {
		return SessionMeta{}, false, nil
	}

	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return SessionMeta{}, false, err
	}
	return meta, true, nil
}

func (m *Manager) loadSessionMetas(sessionIDs []string) ([]SessionMeta, error) {
	metas := make([]SessionMeta, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		meta, found, err := m.loadSessionMeta(sessionID)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

func (m *Manager) formatHistoryEntry(index int, lang string, meta SessionMeta) string {
	aliasStr := ""
	if meta.Alias != "" {
		aliasStr = fmt.Sprintf("\"%s\" ", meta.Alias)
	}

	isActive := ""
	if index == 0 {
		isActive = m.wizard.GetString(lang, "session_history_active")
	}

	shortID := meta.ID
	if len(shortID) > 6 {
		shortID = shortID[:6]
	}

	title := ""
	if meta.RemoteTitle != "" {
		title = " - " + meta.RemoteTitle
	}
	workspaceLabel := ""
	if meta.WorkspaceID != "" {
		workspaceLabel = " @" + meta.WorkspaceID
	}

	return fmt.Sprintf("[%d] %s%s: %s(%s)%s%s\n", index+1, meta.AgentID, workspaceLabel, aliasStr, shortID, isActive, title)
}

func resolveSessionTarget(args string, state ChannelState, metas []SessionMeta) string {
	var idx int
	if _, err := fmt.Sscanf(args, "%d", &idx); err == nil && idx > 0 && idx <= len(state.History) {
		return state.History[idx-1]
	}

	for _, meta := range metas {
		if strings.EqualFold(meta.Alias, args) {
			return meta.ID
		}
	}
	for _, meta := range metas {
		if strings.HasPrefix(meta.ID, args) {
			return meta.ID
		}
	}
	for _, meta := range metas {
		if strings.EqualFold(meta.AgentID, args) && meta.ID != state.ActiveSessionID {
			return meta.ID
		}
	}

	return ""
}

func (m *Manager) formatRemoteHistoryEntry(index int, session middleware.RemoteSessionInfo) string {
	label := session.DisplayID
	if label == "" {
		label = session.RemoteSessionID
	}
	title := ""
	if session.Title != "" {
		title = " - " + session.Title
	}
	status := ""
	if session.Status != "" {
		status = " [" + session.Status + "]"
	}
	return fmt.Sprintf("[R%d] %s%s%s\n", index+1, label, title, status)
}

func (m *Manager) listRemoteSessionsForChannel(ctx context.Context, channelID string) ([]middleware.RemoteSessionInfo, middleware.ConversationSessionCapabilities, error) {
	controller, agentID, err := m.sessionControllerForChannel(channelID)
	if err != nil {
		return nil, middleware.ConversationSessionCapabilities{}, err
	}
	return controller.ListAgentSessions(ctx, agentID)
}

func (m *Manager) sessionControllerForChannel(channelID string) (middleware.AgentSessionController, string, error) {
	controller, ok := m.router.(middleware.AgentSessionController)
	if !ok {
		return nil, "", fmt.Errorf("router does not expose session control")
	}
	state, err := m.getChannelState(channelID)
	if err != nil {
		return nil, "", err
	}
	meta, found, err := m.loadSessionMeta(state.ActiveSessionID)
	if err != nil {
		return nil, "", err
	}
	if !found || strings.TrimSpace(meta.AgentID) == "" {
		return nil, "", fmt.Errorf("active session not found")
	}
	return controller, meta.AgentID, nil
}

func (m *Manager) trySwitchToRemoteSession(ctx context.Context, channelID, target string) (middleware.SessionActionResult, bool, error) {
	controller, agentID, _ := m.sessionControllerForChannel(channelID)
	if controller == nil {
		return middleware.SessionActionResult{}, false, nil
	}
	remoteSessions, _, _ := controller.ListAgentSessions(ctx, agentID)
	match := matchRemoteSessionTarget(target, remoteSessions)
	if match == nil {
		return middleware.SessionActionResult{}, false, nil
	}
	sessionID, err := m.importRemoteSession(channelID, agentID, *match)
	if err != nil {
		return middleware.SessionActionResult{}, true, err
	}
	meta, _, _ := m.loadSessionMeta(sessionID)
	return middleware.SessionActionResult{
		Action:          "switch",
		Message:         fmt.Sprintf("Switched to remote %s session: %s", string(match.ProtocolKind), sessionID),
		ActiveSessionID: sessionID,
		Session:         m.toSessionEntry(meta, true),
		RemoteSessions:  []middleware.RemoteSessionInfo{*match},
	}, true, nil
}

func matchRemoteSessionTarget(target string, sessions []middleware.RemoteSessionInfo) *middleware.RemoteSessionInfo {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	for _, session := range sessions {
		if strings.EqualFold(session.DisplayID, target) || strings.EqualFold(session.RemoteSessionID, target) {
			candidate := session
			return &candidate
		}
	}
	for _, session := range sessions {
		if strings.HasPrefix(session.DisplayID, target) || strings.HasPrefix(session.RemoteSessionID, target) {
			candidate := session
			return &candidate
		}
	}
	for _, session := range sessions {
		if strings.EqualFold(session.Title, target) {
			candidate := session
			return &candidate
		}
	}
	return nil
}

func (m *Manager) importRemoteSession(channelID, agentID string, remote middleware.RemoteSessionInfo) (string, error) {
	keys, err := m.storage.List("session.meta.")
	if err != nil {
		return "", err
	}
	for _, key := range keys {
		id := strings.TrimPrefix(key, "session.meta.")
		meta, found, err := m.loadSessionMeta(id)
		if err != nil || !found {
			continue
		}
		if meta.AgentID == agentID && meta.AgentSessionID == remote.RemoteSessionID {
			if err := m.AttachChannel(channelID, meta.ID); err != nil {
				return "", err
			}
			return meta.ID, nil
		}
	}

	sessionID := uuid.New().String()
	meta := SessionMeta{
		ID:             sessionID,
		AgentSessionID: remote.RemoteSessionID,
		CreatedAt:      time.Now().UTC(),
		AgentID:        agentID,
		Status:         "active",
		ProtocolKind:   string(remote.ProtocolKind),
		MirrorStatus:   "mirrored",
		RemoteTitle:    remote.Title,
		LastSyncedAt:   time.Now().UTC(),
	}
	if state, err := m.getChannelState(channelID); err == nil && strings.TrimSpace(state.PreferredWorkspaceID) != "" {
		if err := m.bindSessionWorkspace(&meta, state.PreferredWorkspaceID, ""); err != nil {
			return "", err
		}
	}
	if remote.UpdatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, remote.UpdatedAt); err == nil {
			meta.RemoteUpdatedAt = parsed
		}
	}
	if err := m.saveSessionMeta(meta); err != nil {
		return "", err
	}
	if err := m.updateChannelState(channelID, sessionID); err != nil {
		return "", err
	}
	if err := m.updateChannelWorkspaceState(channelID, meta.WorkspaceID); err != nil {
		return "", err
	}
	if err := m.indexSessionWorkspace(meta); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (m *Manager) deleteRemoteSession(ctx context.Context, meta SessionMeta) error {
	controller, ok := m.router.(middleware.AgentSessionController)
	if !ok {
		return nil
	}
	if strings.TrimSpace(meta.AgentSessionID) == "" {
		return nil
	}
	return controller.DeleteAgentSession(ctx, meta.AgentID, meta.AgentSessionID)
}

func (m *Manager) cancelRemoteSession(ctx context.Context, meta SessionMeta) error {
	controller, ok := m.router.(middleware.AgentSessionController)
	if !ok {
		return nil
	}
	if strings.TrimSpace(meta.AgentSessionID) == "" {
		return nil
	}
	return controller.CancelAgentSession(ctx, meta.AgentID, meta.AgentSessionID)
}

func (m *Manager) tryDeleteRemoteSession(ctx context.Context, channelID, target string) (middleware.SessionActionResult, bool, error) {
	controller, agentID, _ := m.sessionControllerForChannel(channelID)
	if controller == nil {
		return middleware.SessionActionResult{}, false, nil
	}
	remoteSessions, _, _ := controller.ListAgentSessions(ctx, agentID)
	match := matchRemoteSessionTarget(target, remoteSessions)
	if match == nil {
		return middleware.SessionActionResult{}, false, nil
	}
	if err := controller.DeleteAgentSession(ctx, agentID, match.RemoteSessionID); err != nil {
		return middleware.SessionActionResult{}, true, err
	}
	return middleware.SessionActionResult{
		Action:         "delete",
		Message:        fmt.Sprintf("Deleted remote %s session: %s", string(match.ProtocolKind), match.DisplayID),
		RemoteSessions: []middleware.RemoteSessionInfo{*match},
	}, true, nil
}

func (m *Manager) tryCancelRemoteSession(ctx context.Context, channelID, target string) (middleware.SessionActionResult, bool, error) {
	controller, agentID, _ := m.sessionControllerForChannel(channelID)
	if controller == nil {
		return middleware.SessionActionResult{}, false, nil
	}
	remoteSessions, _, _ := controller.ListAgentSessions(ctx, agentID)
	match := matchRemoteSessionTarget(target, remoteSessions)
	if match == nil {
		return middleware.SessionActionResult{}, false, nil
	}
	if err := controller.CancelAgentSession(ctx, agentID, match.RemoteSessionID); err != nil {
		return middleware.SessionActionResult{}, true, err
	}
	return middleware.SessionActionResult{
		Action:         "cancel",
		Message:        fmt.Sprintf("Canceled remote %s session: %s", string(match.ProtocolKind), match.DisplayID),
		RemoteSessions: []middleware.RemoteSessionInfo{*match},
	}, true, nil
}

func (m *Manager) toSessionEntry(meta SessionMeta, active bool) *middleware.SessionEntry {
	return &middleware.SessionEntry{
		LogicalSessionID: meta.ID,
		RemoteSessionID:  meta.AgentSessionID,
		AgentID:          meta.AgentID,
		Alias:            meta.Alias,
		ProtocolKind:     meta.ProtocolKind,
		WorkspaceID:      meta.WorkspaceID,
		WorkspacePath:    meta.WorkspacePath,
		WorkspaceBranch:  meta.WorkspaceBranch,
		WorkspaceRole:    meta.WorkspaceRole,
		Mode:             normalizeMode(meta.Mode),
		Status:           meta.Status,
		RemoteStatus:     meta.RemoteStatus,
		Title:            meta.RemoteTitle,
		CreatedAt:        meta.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        meta.LastSyncedAt.Format(time.RFC3339),
		Active:           active,
		Meta:             meta.RemoteMeta,
		PendingHandoff:   meta.PendingHandoff,
		LastHandoff:      meta.LastHandoff,
	}
}

func (m *Manager) renderSessionAction(result middleware.SessionActionResult, lang string) string {
	if result.Message != "" && result.Action != "list" {
		return result.Message
	}
	switch result.Action {
	case "status":
		if result.Session == nil {
			return m.wizard.GetString(lang, "session_not_found_db")
		}
		aliasStr := ""
		if result.Session.Alias != "" {
			aliasStr = fmt.Sprintf("\nAlias: \"%s\"", result.Session.Alias)
		}
		if result.Session.WorkspaceID != "" {
			aliasStr += fmt.Sprintf("\nWorkspace: %s", result.Session.WorkspaceID)
			if result.Session.WorkspacePath != "" {
				aliasStr += fmt.Sprintf(" (%s)", result.Session.WorkspacePath)
			}
		}
		if result.Session.Mode != "" {
			aliasStr += fmt.Sprintf("\nMode: %s", result.Session.Mode)
		}
		if result.Session.LastHandoff != nil && result.Session.LastHandoff.FromAgentID != "" {
			aliasStr += fmt.Sprintf("\nHandoff: %s -> %s", result.Session.LastHandoff.FromAgentID, valueOrDash(result.Session.LastHandoff.ToAgentID))
		}
		return fmt.Sprintf(m.wizard.GetString(lang, "session_status"), result.Session.LogicalSessionID, aliasStr, result.Session.AgentID, result.Session.CreatedAt)
	case "list":
		if result.Message != "" && len(result.Sessions) == 0 && len(result.RemoteSessions) == 0 {
			return result.Message
		}
		var sb strings.Builder
		if len(result.Sessions) > 0 {
			sb.WriteString(m.wizard.GetString(lang, "session_history_header") + "\n")
			for i, session := range result.Sessions {
				sb.WriteString(m.formatSessionEntry(i, lang, session))
			}
		}
		if len(result.RemoteSessions) > 0 {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("Remote sessions:\n")
			for i, session := range result.RemoteSessions {
				sb.WriteString(m.formatRemoteHistoryEntry(i, session))
			}
		}
		if sb.Len() == 0 {
			return result.Message
		}
		return sb.String()
	default:
		return result.Message
	}
}

func (m *Manager) formatSessionEntry(index int, lang string, session middleware.SessionEntry) string {
	meta := SessionMeta{
		ID:          session.LogicalSessionID,
		AgentID:     session.AgentID,
		Alias:       session.Alias,
		RemoteTitle: session.Title,
		WorkspaceID: session.WorkspaceID,
	}
	if session.Active {
		return m.formatHistoryEntry(index, lang, meta)
	}
	shortID := meta.ID
	if len(shortID) > 6 {
		shortID = shortID[:6]
	}
	aliasStr := ""
	if meta.Alias != "" {
		aliasStr = fmt.Sprintf("\"%s\" ", meta.Alias)
	}
	title := ""
	if meta.RemoteTitle != "" {
		title = " - " + meta.RemoteTitle
	}
	workspaceLabel := ""
	if session.WorkspaceID != "" {
		workspaceLabel = " @" + session.WorkspaceID
	}
	return fmt.Sprintf("[%d] %s%s: %s(%s)%s\n", index+1, meta.AgentID, workspaceLabel, aliasStr, shortID, title)
}

func (m *Manager) removeSessionMirror(channelID, sessionID string) error {
	channelKeys, err := m.storage.List("session.channel.")
	if err != nil {
		return err
	}
	if len(channelKeys) == 0 {
		channelKeys = []string{getChannelKey(channelID)}
	}
	for _, key := range channelKeys {
		raw, err := m.storage.Get(key)
		if err != nil || len(raw) == 0 {
			continue
		}
		var state ChannelState
		if err := json.Unmarshal(raw, &state); err != nil {
			continue
		}
		nextHistory := make([]string, 0, len(state.History))
		for _, id := range state.History {
			if id != sessionID {
				nextHistory = append(nextHistory, id)
			}
		}
		state.History = nextHistory
		if state.ActiveSessionID == sessionID {
			if len(nextHistory) > 0 {
				state.ActiveSessionID = nextHistory[0]
			} else {
				state.ActiveSessionID = ""
			}
		}
		payload, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if err := m.storage.Set(key, payload); err != nil {
			return err
		}
	}
	return m.storage.Delete(getSessionKey(sessionID))
}
