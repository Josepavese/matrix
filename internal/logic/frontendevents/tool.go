package frontendevents

import (
	"strings"

	"github.com/jose/matrix-v2/internal/logic/runtrace"
)

type ToolEvent struct {
	Name         string
	Kind         string
	Status       string
	Summary      string
	Inputs       map[string]interface{}
	Outputs      map[string]interface{}
	ArtifactRefs []string
}

func NormalizeTool(content string, metadata map[string]interface{}, fallbackName string) ToolEvent {
	name := NormalizeToolName(FirstNonEmpty(
		StringValue(metadata, "tool_name"),
		StringValue(metadata, "name"),
		StringValue(metadata, "title"),
		fallbackName,
	))
	kind := NormalizeToolKind(name, content)
	path := FirstPath(content, metadata)
	operation := ToolOperation(content, metadata)
	status := NormalizeToolStatus(FirstNonEmpty(StringValue(metadata, "status"), statusFromSourceUpdate(metadata)))
	return ToolEvent{
		Name:         name,
		Kind:         kind,
		Status:       status,
		Summary:      toolSummary(toolSummaryContext{Status: status, Name: name, Kind: kind, Path: path, Operation: operation, Content: content}),
		Inputs:       toolInputs(path, operation),
		Outputs:      toolOutputs(path, operation, status),
		ArtifactRefs: toolArtifactRefs(path, kind, status),
	}
}

func EnrichToolWithContext(tool ToolEvent, path, operation string) ToolEvent {
	path = strings.TrimSpace(path)
	operation = strings.TrimSpace(operation)
	if path == "" && operation == "" {
		return tool
	}
	if path != "" {
		tool.Inputs = mergeToolMap(tool.Inputs, map[string]interface{}{"path": path})
		if tool.Status == runtrace.StatusCompleted {
			tool.Outputs = mergeToolMap(tool.Outputs, map[string]interface{}{"path": path})
			if len(tool.ArtifactRefs) == 0 {
				tool.ArtifactRefs = toolArtifactRefs(path, tool.Kind, tool.Status)
			}
		}
	}
	if operation != "" {
		tool.Inputs = mergeToolMap(tool.Inputs, map[string]interface{}{"operation": operation})
		if tool.Status == runtrace.StatusCompleted && tool.Kind == "execute" {
			tool.Outputs = mergeToolMap(tool.Outputs, map[string]interface{}{"operation": operation})
		}
	}
	tool.Summary = toolSummary(toolSummaryContext{
		Status:    tool.Status,
		Name:      tool.Name,
		Kind:      tool.Kind,
		Path:      path,
		Operation: operation,
		Content:   tool.Summary,
	})
	return tool
}

func NormalizeToolName(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.NewReplacer("-", "_", " ", "_").Replace(value)
	for _, rule := range toolNameRules {
		if containsAny(value, rule.matches) {
			return rule.name
		}
	}
	if value == "" {
		return "tool"
	}
	return value
}

func NormalizeToolKind(name, content string) string {
	value := strings.ToLower(name + " " + content)
	for _, rule := range toolKindRules {
		if containsAny(value, rule.matches) {
			return rule.kind
		}
	}
	return "other"
}

func NormalizeToolStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pending", "started", "start":
		return "pending"
	case "running", "progress", "in_progress":
		return "running"
	case "failed", "error":
		return runtrace.StatusFailed
	default:
		return runtrace.StatusCompleted
	}
}

func toolInputs(path, operation string) map[string]interface{} {
	if path == "" && operation == "" {
		return nil
	}
	inputs := map[string]interface{}{}
	if path != "" {
		inputs["path"] = path
	}
	if operation != "" {
		inputs["operation"] = operation
	}
	return inputs
}

func toolOutputs(path, operation, status string) map[string]interface{} {
	if status != runtrace.StatusCompleted || (path == "" && operation == "") {
		return nil
	}
	outputs := map[string]interface{}{}
	if path != "" {
		outputs["path"] = path
	}
	if operation != "" {
		outputs["operation"] = operation
	}
	return outputs
}

func toolArtifactRefs(path, kind, status string) []string {
	if path == "" || status != runtrace.StatusCompleted || !isArtifactKind(kind) {
		return nil
	}
	return []string{"file://" + path}
}

func statusFromSourceUpdate(metadata map[string]interface{}) string {
	if SourceUpdateType(metadata, "") == "tool_call" {
		return "pending"
	}
	return runtrace.StatusCompleted
}

func mergeToolMap(base, extra map[string]interface{}) map[string]interface{} {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = map[string]interface{}{}
	}
	for k, v := range extra {
		if v != "" && v != nil {
			base[k] = v
		}
	}
	return base
}

func containsAny(value string, tokens []string) bool {
	for _, token := range tokens {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

func isArtifactKind(kind string) bool {
	return kind == "edit" || kind == "delete" || kind == "move"
}

type toolNameRule struct {
	name    string
	matches []string
}

var toolNameRules = []toolNameRule{
	{name: "write_file", matches: []string{"write", "create"}},
	{name: "read_file", matches: []string{"read"}},
	{name: "edit_file", matches: []string{"edit", "patch"}},
	{name: "search", matches: []string{"search", "grep"}},
	{name: "list_files", matches: []string{"list", "ls"}},
	{name: "shell", matches: []string{"terminal", "shell", "bash", "exec"}},
}

type toolKindRule struct {
	kind    string
	matches []string
}

var toolKindRules = []toolKindRule{
	{kind: "edit", matches: []string{"write", "create", "edit", "patch"}},
	{kind: "delete", matches: []string{"delete", "remove"}},
	{kind: "move", matches: []string{"move", "rename"}},
	{kind: "search", matches: []string{"search", "grep"}},
	{kind: "read", matches: []string{"read", "list"}},
	{kind: "execute", matches: []string{"shell", "terminal", "exec", "bash"}},
	{kind: "fetch", matches: []string{"fetch", "http"}},
}
