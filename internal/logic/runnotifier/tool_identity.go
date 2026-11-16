package runnotifier

import "github.com/Josepavese/matrix/internal/logic/frontendevents"

// correlatedCall is the identity a result may inherit from the call it belongs
// to. The call window is protocol correlation (same tool_call_id), so a result
// body can never decide the action on its own.
func (n *Notifier) correlatedCall(call toolContext) frontendevents.ToolEvent {
	return frontendevents.ToolEvent{
		Name:                     call.Name,
		Kind:                     call.Kind,
		KindFromIdentity:         call.KindFromIdentity,
		SemanticKind:             call.SemanticKind,
		Effect:                   call.Effect,
		SubjectKind:              call.SubjectKind,
		ClassificationSource:     call.ClassificationSource,
		ClassificationConfidence: call.ClassificationConfidence,
	}
}

// resultCall selects the open call a result belongs to. A result that carries
// its own peer-assigned call id is correlated by that id even while another call
// is the most recent request; the last-request slot is only the fallback for a
// peer update without an explicit identity.
func (n *Notifier) resultCall(tool frontendevents.ToolEvent, content string, metadata map[string]interface{}) (string, toolContext, bool) {
	toolID := n.activeToolCallID
	if frontendevents.ToolCallID(metadata) != "" {
		toolID = frontendevents.StableToolCallID(n.runID, tool.Name, content, metadata)
	}
	if toolID == "" {
		toolID = frontendevents.StableToolCallID(n.runID, tool.Name, content, metadata)
	}
	if correlated, ok := n.toolCalls[toolID]; ok {
		return toolID, correlated, true
	}
	if toolID == n.activeTool.ToolID {
		return toolID, n.activeTool, true
	}
	return toolID, toolContext{}, false
}
