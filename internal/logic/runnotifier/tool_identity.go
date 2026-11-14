package runnotifier

import "github.com/Josepavese/matrix/internal/logic/frontendevents"

// correlatedCall is the identity a result may inherit from the call it belongs
// to. The call window is protocol correlation (same tool_call_id), so a result
// body can never decide the action on its own.
func (n *Notifier) correlatedCall() frontendevents.ToolEvent {
	return frontendevents.ToolEvent{
		Name:                     n.activeTool.Name,
		Kind:                     n.activeTool.Kind,
		KindFromIdentity:         n.activeTool.KindFromIdentity,
		SemanticKind:             n.activeTool.SemanticKind,
		Effect:                   n.activeTool.Effect,
		SubjectKind:              n.activeTool.SubjectKind,
		ClassificationSource:     n.activeTool.ClassificationSource,
		ClassificationConfidence: n.activeTool.ClassificationConfidence,
	}
}
