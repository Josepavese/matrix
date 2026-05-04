package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/sessionview"
	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/google/uuid"
)

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

func (m *Manager) handleSessionListTyped(ctx context.Context, req middleware.SessionActionRequest, lang string) (middleware.SessionActionResult, error) {
	state, err := m.getChannelState(req.ChannelID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}

	metas, err := m.loadSessionMetas(state.History)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	local := make([]middleware.SessionEntry, 0, len(metas))
	for _, meta := range metas {
		if req.WorkspaceID == "" || meta.WorkspaceID == req.WorkspaceID {
			local = append(local, *m.toSessionEntry(meta, meta.ID == state.ActiveSessionID))
		}
	}

	remote, _, remoteErr := []middleware.RemoteSessionInfo(nil), middleware.ConversationSessionCapabilities{}, error(nil)
	if !req.LocalOnly {
		remote, _, remoteErr = m.listRemoteSessionsForChannel(ctx, req.ChannelID)
	}
	result := middleware.SessionActionResult{Action: "list", ActiveSessionID: state.ActiveSessionID, Sessions: local, RemoteSessions: remote}
	if len(local) == 0 && len(remote) == 0 {
		result.Message = m.wizard.GetString(lang, "session_history_empty")
		if remoteErr != nil {
			result.Message = fmt.Sprintf("%s\nRemote discovery unavailable: %v", result.Message, remoteErr)
		}
	}
	return result, nil
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

func (m *Manager) handleSessionCancelTyped(ctx context.Context, channelID, lang, args string) (middleware.SessionActionResult, error) {
	targetID, err := m.resolveSessionCancelTarget(channelID, args)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if targetID == "" {
		return middleware.SessionActionResult{Action: "cancel", Message: m.wizard.GetString(lang, "session_history_empty")}, nil
	}

	meta, found, err := m.loadSessionMeta(targetID)
	if err != nil {
		return middleware.SessionActionResult{}, err
	}
	if !found {
		if canceled, handled, err := m.tryCancelRemoteSession(ctx, channelID, targetID); handled {
			return canceled, err
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

func (m *Manager) resolveSessionCancelTarget(channelID, args string) (string, error) {
	state, err := m.getChannelState(channelID)
	if err != nil {
		return "", err
	}
	targetID := firstNonEmpty(strings.TrimSpace(args), state.ActiveSessionID)
	if targetID == "" {
		return "", nil
	}
	metas, err := m.loadSessionMetas(state.History)
	if err != nil {
		return "", err
	}
	if resolved := resolveSessionTarget(targetID, state, metas); resolved != "" {
		targetID = resolved
	}
	return targetID, nil
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

func resolveSessionTarget(args string, state ChannelState, metas []SessionMeta) string {
	if idx, err := strconv.Atoi(args); err == nil && idx > 0 && idx <= len(state.History) {
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
	existingID, err := m.findRemoteSessionMirror(agentID, remote.RemoteSessionID)
	if err != nil {
		return "", err
	}
	if existingID != "" {
		if err := m.AttachChannel(channelID, existingID); err != nil {
			return "", err
		}
		return existingID, nil
	}

	meta, err := m.importedRemoteSessionMeta(channelID, agentID, remote)
	if err != nil {
		return "", err
	}
	return m.persistImportedRemoteSession(channelID, meta)
}

func (m *Manager) findRemoteSessionMirror(agentID, remoteSessionID string) (string, error) {
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
		if meta.AgentID == agentID && meta.AgentSessionID == remoteSessionID {
			return meta.ID, nil
		}
	}
	return "", nil
}

func (m *Manager) importedRemoteSessionMeta(channelID, agentID string, remote middleware.RemoteSessionInfo) (SessionMeta, error) {
	now := time.Now().UTC()
	meta := SessionMeta{
		ID:             uuid.New().String(),
		AgentSessionID: remote.RemoteSessionID,
		CreatedAt:      now,
		AgentID:        agentID,
		Status:         "active",
		ProtocolKind:   string(remote.ProtocolKind),
		MirrorStatus:   "mirrored",
		RemoteTitle:    remote.Title,
		LastSyncedAt:   now,
	}
	if parsed, ok := parseRemoteUpdatedAt(remote.UpdatedAt); ok {
		meta.RemoteUpdatedAt = parsed
	}
	if err := m.applyPreferredWorkspace(channelID, &meta); err != nil {
		return SessionMeta{}, err
	}
	return meta, nil
}

func (m *Manager) applyPreferredWorkspace(channelID string, meta *SessionMeta) error {
	if state, err := m.getChannelState(channelID); err == nil && strings.TrimSpace(state.PreferredWorkspaceID) != "" {
		return m.bindSessionWorkspace(meta, state.PreferredWorkspaceID, "")
	}
	return nil
}

func (m *Manager) persistImportedRemoteSession(channelID string, meta SessionMeta) (string, error) {
	if err := m.saveSessionMeta(meta); err != nil {
		return "", err
	}
	if err := m.updateChannelState(channelID, meta.ID); err != nil {
		return "", err
	}
	if err := m.updateChannelWorkspaceState(channelID, meta.WorkspaceID); err != nil {
		return "", err
	}
	if err := m.indexSessionWorkspace(meta); err != nil {
		return "", err
	}
	return meta.ID, nil
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
		Ephemeral:        meta.Ephemeral,
		CleanupPolicy:    meta.CleanupPolicy,
		Meta:             meta.RemoteMeta,
		PendingHandoff:   meta.PendingHandoff,
		LastHandoff:      meta.LastHandoff,
		ParentSessionID:  meta.ParentSessionID,
		ParentRemoteID:   meta.ParentRemoteID,
	}
}

func (m *Manager) renderSessionAction(result middleware.SessionActionResult, lang string) string {
	lookup := func(key string) string {
		return m.wizard.GetString(lang, key)
	}
	return sessionview.RenderAction(result, lang, sessionview.RenderDeps{
		Lookup: lookup,
		Local:  m.formatSessionEntry,
		Remote: m.formatRemoteHistoryEntry,
	})
}

func (m *Manager) formatSessionEntry(index int, lang string, session middleware.SessionEntry) string {
	shortID := session.LogicalSessionID
	if len(shortID) > 6 {
		shortID = shortID[:6]
	}
	aliasStr := ""
	if session.Alias != "" {
		aliasStr = fmt.Sprintf("\"%s\" ", session.Alias)
	}
	title := ""
	if session.Title != "" {
		title = " - " + session.Title
	}
	workspaceLabel := ""
	if session.WorkspaceID != "" {
		workspaceLabel = " @" + session.WorkspaceID
	}
	active := ""
	if session.Active {
		active = m.wizard.GetString(lang, "session_history_active")
	}
	return fmt.Sprintf("[%d] %s%s: %s(%s)%s%s\n", index+1, session.AgentID, workspaceLabel, aliasStr, shortID, active, title)
}

func (m *Manager) removeSessionMirror(channelID, sessionID string) error {
	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil {
		return err
	}
	channelKeys, err := m.channelKeysForSessionRemoval(channelID)
	if err != nil {
		return err
	}
	for _, key := range channelKeys {
		if err := m.removeSessionFromChannelState(key, sessionID); err != nil {
			return err
		}
	}
	if err := m.removeWorkspaceSessionIndex(meta, found, sessionID); err != nil {
		return err
	}
	return m.storage.Delete(getSessionKey(sessionID))
}

func (m *Manager) channelKeysForSessionRemoval(channelID string) ([]string, error) {
	channelKeys, err := m.storage.List("session.channel.")
	if err != nil {
		return nil, err
	}
	if len(channelKeys) == 0 {
		return []string{getChannelKey(channelID)}, nil
	}
	return channelKeys, nil
}

func (m *Manager) removeSessionFromChannelState(key, sessionID string) error {
	state, found := m.loadChannelStateByKey(key)
	if !found {
		return nil
	}
	state.History = removeSessionID(state.History, sessionID)
	if state.ActiveSessionID == sessionID {
		state.ActiveSessionID = firstSessionID(state.History)
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return m.storage.Set(key, payload)
}

func (m *Manager) loadChannelStateByKey(key string) (ChannelState, bool) {
	raw, err := m.storage.Get(key)
	if err != nil {
		return ChannelState{}, false
	}
	if len(raw) == 0 {
		return ChannelState{}, false
	}
	var state ChannelState
	if err := json.Unmarshal(raw, &state); err != nil {
		return ChannelState{}, false
	}
	return state, true
}

func removeSessionID(history []string, sessionID string) []string {
	nextHistory := make([]string, 0, len(history))
	for _, id := range history {
		if id != sessionID {
			nextHistory = append(nextHistory, id)
		}
	}
	return nextHistory
}

func firstSessionID(history []string) string {
	if len(history) == 0 {
		return ""
	}
	return history[0]
}

func (m *Manager) removeWorkspaceSessionIndex(meta SessionMeta, found bool, sessionID string) error {
	if !found || strings.TrimSpace(meta.WorkspaceID) == "" {
		return nil
	}
	return workspace.RemoveSessionIndex(m.storage, meta.WorkspaceID, sessionID)
}
