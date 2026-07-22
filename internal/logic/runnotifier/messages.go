package runnotifier

import (
	"github.com/Josepavese/matrix/internal/logic/frontendevents"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

type messageEventSpec struct {
	kind, status, classification  string
	frontendVisible, auditVisible bool
}

func (n *Notifier) appendMessageUpdate(update middleware.ThoughtUpdate) {
	classification := frontendevents.StringValue(update.Metadata, "message_classification")
	if classification == "" {
		classification = "unclassified"
	}
	spec := messageEventSpec{kind: "agent.message.delta", status: "streaming", classification: classification, frontendVisible: true}
	switch classification {
	case "progress":
		spec.kind = "agent.message.progress"
	case "final":
		n.sawExplicitFinal = true
	case "unclassified":
		if n.sawExplicitFinal {
			spec = messageEventSpec{kind: "agent.runtime.diagnostic", status: runtrace.StatusCompleted, classification: "diagnostic_after_final", auditVisible: true}
		}
	}
	n.appendMessageEvent(update, spec)
}

func (n *Notifier) appendMessageEvent(update middleware.ThoughtUpdate, spec messageEventSpec) {
	event := n.baseEvent(spec.kind, n.agentID, spec.status, update.Content)
	event.ContentRef = "matrix://runs/" + n.runID + "/messages/" + spec.classification
	event.ProtocolMethod = "session/update"
	event.Message = update.Content
	event.Metadata = frontendevents.Merge(event.Metadata, map[string]interface{}{
		"source_update_type":     frontendevents.SourceUpdateType(update.Metadata, "agent_message_chunk"),
		"message_id":             frontendevents.StringValue(update.Metadata, "message_id"),
		"message_phase":          frontendevents.StringValue(update.Metadata, "message_phase"),
		"message_classification": spec.classification,
		"frontend_visible":       spec.frontendVisible,
		"audit_visible":          spec.auditVisible,
	})
	event.ProtocolMeta = frontendevents.ProtocolMeta(update.Metadata)
	_, _ = n.store.AppendEvent(event)
}
