// Package runnotifier projects live agent thought updates into run traces.
package runnotifier

import (
	"log/slog"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/frontendevents"
	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/middleware"
)

type Notifier struct {
	store            *runtrace.Store
	runID            string
	agentID          string
	protocol         string
	activeToolCallID string
	activeTool       toolContext
}

type toolContext struct {
	EventID                  string
	ToolID                   string
	Name                     string
	Kind                     string
	SemanticKind             string
	Effect                   string
	SubjectKind              string
	ClassificationSource     string
	ClassificationConfidence string
	Path                     string
	Operation                string
}

func New(store *runtrace.Store, runID, agentID, protocol string) *Notifier {
	return &Notifier{store: store, runID: runID, agentID: agentID, protocol: protocol}
}

func (n *Notifier) OnThought(update middleware.ThoughtUpdate) {
	if n == nil || n.store == nil {
		return
	}
	switch update.Type {
	case middleware.ThoughtTypeToolCall:
		n.appendToolRequested(update)
	case middleware.ThoughtTypeToolResult:
		n.appendToolResult(update)
	case middleware.ThoughtTypePermission:
		n.appendPermission(update)
	default:
		n.append("agent.message.delta", n.agentID, "streaming", update.Content)
	}
}

func (n *Notifier) SetHeader(agentID, remoteSessionID string) {
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

func (n *Notifier) SetLogicalSession(logicalSessionID, workspaceID string) {
	if n == nil || n.store == nil {
		return
	}
	run, found, err := n.store.LoadRun(n.runID)
	if err != nil || !found {
		return
	}
	if strings.TrimSpace(logicalSessionID) != "" {
		run.LogicalSessionID = strings.TrimSpace(logicalSessionID)
	}
	if strings.TrimSpace(workspaceID) != "" {
		run.WorkspaceID = strings.TrimSpace(workspaceID)
	}
	if err := n.store.SaveRun(run); err != nil {
		slog.Warn("failed to update run logical session metadata", "error", err, "run_id", n.runID)
	}
}

func (n *Notifier) FormattedHeader() string {
	return ""
}

func (n *Notifier) append(kind, actor, status, content string) {
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

func (n *Notifier) appendToolRequested(update middleware.ThoughtUpdate) {
	content := strings.TrimSpace(frontendevents.FirstNonEmpty(update.Content, update.Title))
	tool := frontendevents.NormalizeTool(content, update.Metadata, inferToolName(content))
	toolID := frontendevents.StableToolCallID(n.runID, tool.Name, content, update.Metadata)
	n.activeToolCallID = toolID
	event := n.baseEvent("tool.call.requested", "agent", "pending", content)
	event.ProtocolMethod = "session/update"
	event.ToolCallID = toolID
	event.ToolName = tool.Name
	event.ToolKind = tool.Kind
	event.ToolSemanticKind = tool.SemanticKind
	event.ToolEffect = tool.Effect
	event.ToolSubjectKind = tool.SubjectKind
	event.ToolClassificationSource = tool.ClassificationSource
	event.ToolClassificationConfidence = tool.ClassificationConfidence
	event.Summary = tool.Summary
	event.Inputs = tool.Inputs
	event.Metadata = frontendevents.Merge(event.Metadata, map[string]interface{}{
		"source_update_type": frontendevents.SourceUpdateType(update.Metadata, "tool_call"),
		"frontend_visible":   true,
	})
	event.ProtocolMeta = frontendevents.ProtocolMeta(update.Metadata)
	stored, _ := n.store.AppendEvent(event)
	n.activeTool = toolContext{
		EventID:                  stored.ID,
		ToolID:                   toolID,
		Name:                     tool.Name,
		Kind:                     tool.Kind,
		SemanticKind:             tool.SemanticKind,
		Effect:                   tool.Effect,
		SubjectKind:              tool.SubjectKind,
		ClassificationSource:     tool.ClassificationSource,
		ClassificationConfidence: tool.ClassificationConfidence,
		Path:                     stringFromMap(tool.Inputs, "path"),
		Operation:                stringFromMap(tool.Inputs, "operation"),
	}
}

func (n *Notifier) appendToolResult(update middleware.ThoughtUpdate) {
	content := strings.TrimSpace(frontendevents.FirstNonEmpty(update.Content, update.Title))
	tool := frontendevents.NormalizeTool(content, update.Metadata, inferToolName(content))
	toolID := frontendevents.FirstNonEmpty(n.activeToolCallID, frontendevents.StableToolCallID(n.runID, tool.Name, content, update.Metadata))
	if toolID == n.activeTool.ToolID {
		tool = frontendevents.EnrichToolWithContext(tool, n.activeTool.Path, n.activeTool.Operation)
	}
	event := n.baseEvent("tool.result.received", "tool", tool.Status, content)
	event.ProtocolMethod = "session/update"
	event.ToolCallID = toolID
	event.ToolName = tool.Name
	event.ToolKind = tool.Kind
	event.ToolSemanticKind = tool.SemanticKind
	event.ToolEffect = tool.Effect
	event.ToolSubjectKind = tool.SubjectKind
	event.ToolClassificationSource = tool.ClassificationSource
	event.ToolClassificationConfidence = tool.ClassificationConfidence
	event.Summary = tool.Summary
	event.Inputs = tool.Inputs
	event.Outputs = tool.Outputs
	event.ArtifactRefs = tool.ArtifactRefs
	event.Metadata = frontendevents.Merge(event.Metadata, map[string]interface{}{
		"source_update_type": frontendevents.SourceUpdateType(update.Metadata, "tool_call_update"),
		"frontend_visible":   true,
	})
	event.ProtocolMeta = frontendevents.ProtocolMeta(update.Metadata)
	if event.Status == runtrace.StatusCompleted || event.Status == runtrace.StatusFailed {
		n.activeToolCallID = ""
		n.activeTool = toolContext{}
	}
	_, _ = n.store.AppendEvent(event)
}

func (n *Notifier) appendPermission(update middleware.ThoughtUpdate) {
	content := strings.TrimSpace(update.Content)
	permission := frontendevents.NormalizePermission(n.runID, content, update.Metadata)
	n.enrichActiveToolFromPermission(permission)
	requested := n.baseEvent("permission.requested", "agent", "pending", content)
	requested.ProtocolMethod = permission.ProtocolMethod
	requested.PermissionID = permission.ID
	requested.Summary = permission.RequestSummary
	requested.Inputs = permission.RequestInputs
	requested.Metadata = frontendevents.Merge(requested.Metadata, map[string]interface{}{"frontend_visible": false, "audit_visible": true})
	requested.ProtocolMeta = frontendevents.ProtocolMeta(update.Metadata)
	_, _ = n.store.AppendEvent(requested)

	resolved := n.baseEvent("permission.resolved", "matrix", runtrace.StatusCompleted, content)
	resolved.ProtocolMethod = permission.ProtocolMethod
	resolved.PermissionID = permission.ID
	resolved.Summary = permission.ResolutionSummary
	resolved.Outputs = permission.ResolutionOutputs
	resolved.Metadata = frontendevents.Merge(resolved.Metadata, map[string]interface{}{
		"frontend_visible": false,
		"audit_visible":    true,
		"approval_mode":    permission.ApprovalMode,
	})
	resolved.ProtocolMeta = frontendevents.ProtocolMeta(update.Metadata)
	_, _ = n.store.AppendEvent(resolved)
}

func (n *Notifier) enrichActiveToolFromPermission(permission frontendevents.PermissionEvent) {
	if n.activeTool.ToolID == "" {
		return
	}
	path := frontendevents.FirstNonEmpty(n.activeTool.Path, stringFromMap(permission.RequestInputs, "path"))
	operation := frontendevents.FirstNonEmpty(n.activeTool.Operation, stringFromMap(permission.RequestInputs, "operation"))
	if operation == "" {
		operation = stringFromMap(permission.ResolutionOutputs, "operation")
	}
	if path == "" && operation == "" {
		return
	}
	n.activeTool.Path = path
	n.activeTool.Operation = operation
	if n.activeTool.EventID == "" {
		return
	}
	event, found, err := n.store.LoadEvent(n.runID, n.activeTool.EventID)
	if err != nil || !found {
		return
	}
	tool := frontendevents.ToolEvent{
		Name:         event.ToolName,
		Kind:         event.ToolKind,
		Status:       event.Status,
		Summary:      event.Summary,
		Inputs:       event.Inputs,
		Outputs:      event.Outputs,
		ArtifactRefs: event.ArtifactRefs,
	}
	tool = frontendevents.EnrichToolWithContext(tool, path, operation)
	event.Summary = tool.Summary
	event.Inputs = tool.Inputs
	event.Outputs = tool.Outputs
	event.ArtifactRefs = tool.ArtifactRefs
	event.Metadata = frontendevents.Merge(event.Metadata, map[string]interface{}{"enriched_from_permission": true})
	_, _ = n.store.AppendEvent(event)
}

func stringFromMap(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func (n *Notifier) baseEvent(kind, actor, status, content string) runtrace.Event {
	return runtrace.Event{
		RunID:         n.runID,
		Kind:          kind,
		Actor:         frontendevents.FirstNonEmpty(actor, n.agentID, "agent"),
		Status:        status,
		Protocol:      n.protocol,
		ContentDigest: runtrace.DigestString(content),
		Metadata: map[string]interface{}{
			"content_length": len(content),
		},
	}
}
