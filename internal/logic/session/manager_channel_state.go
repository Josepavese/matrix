package session

import (
	"encoding/json"
	"fmt"
	"strings"
)

func historyPushInactive(history []string, activeID, newID string) []string {
	if strings.TrimSpace(activeID) == "" || activeID == newID {
		return historyPush(history, newID)
	}
	filtered := make([]string, 0, len(history))
	for _, id := range history {
		if id != activeID && id != newID {
			filtered = append(filtered, id)
		}
	}
	res := append([]string{activeID, newID}, filtered...)
	if len(res) > 10 {
		res = res[:10]
	}
	return res
}

func (m *Manager) appendInactiveChannelSession(channelID, sessionID string) error {
	state, err := m.getChannelState(channelID)
	if err != nil {
		return err
	}
	state.History = historyPushInactive(state.History, state.ActiveSessionID, sessionID)
	newStateData, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal channel state: %w", err)
	}
	return m.storage.Set(getChannelKey(channelID), newStateData)
}
