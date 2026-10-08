package session

import (
	"log/slog"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// remoteBindingNotifier persists a provider-confirmed remote identity before a
// long prompt returns. Cancellation cannot otherwise recover an identity that
// was visible in the run trace but still pending in the ordered result queue.
type remoteBindingNotifier struct {
	inner middleware.ThoughtNotifier
	bind  func(string, string)
}

func (m *Manager) remoteBinding(sessionID string, inner middleware.ThoughtNotifier) middleware.ThoughtNotifier {
	return &remoteBindingNotifier{inner: inner, bind: func(agentID, remoteID string) {
		meta, found, err := m.loadSessionMeta(sessionID)
		if err != nil || !found || strings.TrimSpace(remoteID) == "" || meta.AgentID != agentID {
			return
		}
		if !applyAgentSessionMirrorID(&meta, remoteID) {
			return
		}
		if err := m.saveSessionMeta(meta); err != nil {
			slog.Warn("failed to persist live remote session binding", "logical_session", sessionID, "error", err)
		}
	}}
}

func (n *remoteBindingNotifier) SetHeader(agentID, remoteID string) {
	n.bind(agentID, remoteID)
	if n.inner != nil {
		n.inner.SetHeader(agentID, remoteID)
	}
}

func (n *remoteBindingNotifier) OnThought(update middleware.ThoughtUpdate) {
	if n.inner != nil {
		n.inner.OnThought(update)
	}
}

func (n *remoteBindingNotifier) FormattedHeader() string {
	if n.inner != nil {
		return n.inner.FormattedHeader()
	}
	return ""
}

func (n *remoteBindingNotifier) OnModelSelection(selection middleware.ModelSelection) {
	if forwarder, ok := n.inner.(middleware.ModelSelectionNotifier); ok {
		forwarder.OnModelSelection(selection)
	}
}

func (n *remoteBindingNotifier) OnTurnStopReason(reason string) {
	if forwarder, ok := n.inner.(interface{ OnTurnStopReason(string) }); ok {
		forwarder.OnTurnStopReason(reason)
	}
}
