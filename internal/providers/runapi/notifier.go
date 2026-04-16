package runapi

import (
	"log/slog"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/middleware"
)

type runTraceNotifier struct {
	store    *runtrace.Store
	runID    string
	agentID  string
	protocol string
}

func (n *runTraceNotifier) OnThought(update middleware.ThoughtUpdate) {
	if n == nil || n.store == nil {
		return
	}
	switch update.Type {
	case middleware.ThoughtTypeToolCall:
		n.append("tool.call.requested", "agent", "started", update.Content)
	case middleware.ThoughtTypeToolResult:
		n.append("tool.result.received", "matrix", runtrace.StatusCompleted, update.Content)
	default:
		n.append("agent.message.delta", n.agentID, "streaming", update.Content)
	}
}

func (n *runTraceNotifier) SetHeader(agentID, remoteSessionID string) {
	if n == nil || n.store == nil {
		return
	}
	if strings.TrimSpace(agentID) != "" {
		n.agentID = agentID
	}
	run, found, err := n.store.LoadRun(n.runID)
	if err != nil || !found {
		return
	}
	if strings.TrimSpace(agentID) != "" {
		run.AgentID = agentID
	}
	if strings.TrimSpace(remoteSessionID) != "" {
		run.RemoteSessionID = remoteSessionID
	}
	if strings.TrimSpace(n.protocol) != "" {
		run.Protocol = n.protocol
	}
	if err := n.store.SaveRun(run); err != nil {
		slog.Warn("failed to update run trace header metadata", "error", err, "run_id", n.runID)
	}
}

func (n *runTraceNotifier) FormattedHeader() string {
	return ""
}

func (n *runTraceNotifier) append(kind, actor, status, content string) {
	content = strings.TrimSpace(content)
	event := n.baseEvent(kind, actor, status, content)
	switch kind {
	case "tool.call.requested":
		event.ToolName = inferToolName(content)
		event.ProtocolMethod = "session/update"
	case "tool.result.received":
		event.ToolName = inferToolName(content)
		event.ProtocolMethod = "session/update"
		event.ArtifactRefs = []string{"matrix://runs/" + n.runID + "/tools/" + event.Kind}
	default:
		event.ContentRef = "matrix://runs/" + n.runID + "/messages/delta"
		event.ProtocolMethod = "session/update"
	}
	_, _ = n.store.AppendEvent(event)
}

func (n *runTraceNotifier) baseEvent(kind, actor, status, content string) runtrace.Event {
	return runtrace.Event{
		RunID:         n.runID,
		Kind:          kind,
		Actor:         firstNonEmpty(actor, n.agentID, "agent"),
		Status:        status,
		Protocol:      n.protocol,
		ContentDigest: runtrace.DigestString(content),
		Metadata: map[string]interface{}{
			"content_length": len(content),
		},
	}
}
