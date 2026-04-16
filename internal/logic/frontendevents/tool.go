package frontendevents

import (
	"fmt"
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
	status := NormalizeToolStatus(FirstNonEmpty(StringValue(metadata, "status"), statusFromSourceUpdate(metadata)))
	return ToolEvent{
		Name:         name,
		Kind:         kind,
		Status:       status,
		Summary:      toolSummary(status, name, path, content),
		Inputs:       toolInputs(path),
		Outputs:      toolOutputs(path, status),
		ArtifactRefs: toolArtifactRefs(path, kind, status),
	}
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

func toolInputs(path string) map[string]interface{} {
	if path == "" {
		return nil
	}
	return map[string]interface{}{"path": path}
}

func toolOutputs(path, status string) map[string]interface{} {
	if path == "" || status != runtrace.StatusCompleted {
		return nil
	}
	return map[string]interface{}{"path": path}
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

func toolSummary(status, name, path, content string) string {
	action := map[string]string{"pending": "Start", "running": "Run", runtrace.StatusCompleted: "Completed", runtrace.StatusFailed: "Failed"}[status]
	if action == "" {
		action = "Run"
	}
	if path != "" {
		return fmt.Sprintf("%s %s on %s", action, name, path)
	}
	if content != "" {
		return truncateSummary(content)
	}
	return fmt.Sprintf("%s %s", action, name)
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
