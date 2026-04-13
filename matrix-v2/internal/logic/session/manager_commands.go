package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (m *Manager) handleSessionStatus(channelID, lang string) (string, error) {
	sessionID, err := m.GetOrCreateSession(channelID, m.defaultAgent)
	if err != nil {
		return "", err
	}

	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil {
		return "", err
	}
	if !found {
		return m.wizard.GetString(lang, "session_not_found_db"), nil
	}

	aliasStr := ""
	if meta.Alias != "" {
		aliasStr = fmt.Sprintf("\nAlias: \"%s\"", meta.Alias)
	}

	return fmt.Sprintf(
		m.wizard.GetString(lang, "session_status"),
		meta.ID,
		aliasStr,
		meta.AgentID,
		meta.CreatedAt.Format(time.RFC3339),
	), nil
}

func (m *Manager) handleSessionName(channelID, lang, alias string) (string, error) {
	if alias == "" {
		return m.wizard.GetString(lang, "session_name_usage"), nil
	}

	sessionID, err := m.GetOrCreateSession(channelID, m.defaultAgent)
	if err != nil {
		return "", err
	}

	meta, found, err := m.loadSessionMeta(sessionID)
	if err != nil {
		return "", err
	}
	if !found {
		return m.wizard.GetString(lang, "session_not_found_db"), nil
	}

	meta.Alias = alias
	newData, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	if err := m.storage.Set(getSessionKey(sessionID), newData); err != nil {
		return "", err
	}

	return fmt.Sprintf(m.wizard.GetString(lang, "session_alias_set"), alias), nil
}

func (m *Manager) handleSessionNew(channelID, lang string, parts []string) (string, error) {
	agentID := m.defaultAgent
	if len(parts) >= 3 {
		agentID = parts[2]
	}

	sessionID, err := m.forceNewSession(channelID, agentID)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(m.wizard.GetString(lang, "session_new_started"), agentID, sessionID), nil
}

func (m *Manager) handleSessionList(channelID, lang string) (string, error) {
	state, err := m.getChannelState(channelID)
	if err != nil {
		return "", err
	}
	if len(state.History) == 0 {
		return m.wizard.GetString(lang, "session_history_empty"), nil
	}

	metas, err := m.loadSessionMetas(state.History)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(m.wizard.GetString(lang, "session_history_header") + "\n")
	for i, meta := range metas {
		sb.WriteString(m.formatHistoryEntry(i, lang, meta))
	}
	return sb.String(), nil
}

func (m *Manager) handleSessionSwitch(channelID, lang, args string) (string, error) {
	state, err := m.getChannelState(channelID)
	if err != nil {
		return "", err
	}
	if len(state.History) == 0 {
		return "No session history to switch to.", nil
	}

	if args == "" {
		return m.switchToPreviousSession(channelID, lang, state)
	}

	metas, err := m.loadSessionMetas(state.History)
	if err != nil {
		return "", err
	}

	targetID := resolveSessionTarget(args, state, metas)
	if targetID == "" {
		return m.createRequestedAgentSession(channelID, lang, args)
	}
	if err := m.AttachChannel(channelID, targetID); err != nil {
		return "", err
	}

	return fmt.Sprintf(m.wizard.GetString(lang, "session_switched"), targetID), nil
}

func (m *Manager) switchToPreviousSession(channelID, lang string, state ChannelState) (string, error) {
	if len(state.History) <= 1 {
		return m.wizard.GetString(lang, "session_history_switch_no_prev"), nil
	}
	if err := m.AttachChannel(channelID, state.History[1]); err != nil {
		return "", err
	}
	return m.wizard.GetString(lang, "session_history_switch_prev"), nil
}

func (m *Manager) createRequestedAgentSession(channelID, lang, args string) (string, error) {
	requestedAgentID := strings.Fields(args)[0]
	sessionID, err := m.forceNewSession(channelID, requestedAgentID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(m.wizard.GetString(lang, "session_switch_resolve_fail_new"), requestedAgentID, sessionID), nil
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

	return fmt.Sprintf("[%d] %s: %s(%s)%s\n", index+1, meta.AgentID, aliasStr, shortID, isActive)
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
