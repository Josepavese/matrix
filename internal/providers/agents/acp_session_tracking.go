package agents

import "strings"

func (c *acpConversationClient) TrackedRemoteSessionIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.loadedSessions))
	for remoteSessionID := range c.loadedSessions {
		if strings.TrimSpace(remoteSessionID) != "" {
			out = append(out, remoteSessionID)
		}
	}
	return out
}
