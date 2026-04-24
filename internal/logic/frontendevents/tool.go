package frontendevents

import (
	"strings"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

type ToolEvent struct {
	Name                     string
	Kind                     string
	SemanticKind             string
	Effect                   string
	SubjectKind              string
	ClassificationSource     string
	ClassificationConfidence string
	Status                   string
	Summary                  string
	Inputs                   map[string]interface{}
	Outputs                  map[string]interface{}
	ArtifactRefs             []string
}

func NormalizeTool(content string, metadata map[string]interface{}, fallbackName string) ToolEvent {
	name := NormalizeToolName(FirstNonEmpty(
		StringValue(metadata, "tool_name"),
		StringValue(metadata, "name"),
		StringValue(metadata, "title"),
		fallbackName,
	))
	kind, classificationSource, confidence := NormalizeToolKindFromMetadata(metadata, name, content)
	path := FirstPath(content, metadata)
	semanticKind := FirstNonEmpty(StringValue(metadata, "tool_semantic_kind"), StringValue(metadata, "semantic_kind"))
	effect := FirstNonEmpty(StringValue(metadata, "tool_effect"), effectForKind(kind))
	subjectKind := FirstNonEmpty(StringValue(metadata, "tool_subject_kind"), subjectForKind(kind, metadata, path))
	operation := ToolOperation(content, metadata)
	status := NormalizeToolStatus(FirstNonEmpty(StringValue(metadata, "status"), statusFromSourceUpdate(metadata)))
	return ToolEvent{
		Name:                     name,
		Kind:                     kind,
		SemanticKind:             semanticKind,
		Effect:                   effect,
		SubjectKind:              subjectKind,
		ClassificationSource:     classificationSource,
		ClassificationConfidence: confidence,
		Status:                   status,
		Summary:                  toolSummary(toolSummaryContext{Status: status, Name: name, Kind: kind, Path: path, Operation: operation, Content: content}),
		Inputs:                   toolInputs(path, operation),
		Outputs:                  toolOutputs(path, operation, status),
		ArtifactRefs:             toolArtifactRefs(path, kind, status),
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
		tool.SubjectKind = "filesystem"
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
	kind, _, _ := fallbackToolKind(name, content)
	return kind
}

func NormalizeToolKindFromMetadata(metadata map[string]interface{}, name, content string) (string, string, string) {
	for _, key := range []string{"tool_kind", "acp_tool_kind", "kind"} {
		if kind := NormalizeOfficialToolKind(StringValue(metadata, key)); kind != "" {
			return kind, "protocol_metadata", "high"
		}
	}
	if kind := nestedACPToolKind(metadata); kind != "" {
		return kind, "protocol_metadata", "high"
	}
	return fallbackToolKind(name, content)
}

func NormalizeOfficialToolKind(raw string) string {
	return officialToolKindAliases[strings.ToLower(strings.TrimSpace(raw))]
}

func fallbackToolKind(name, content string) (string, string, string) {
	value := strings.ToLower(name + " " + content)
	for _, rule := range toolKindRules {
		if containsAny(value, rule.matches) {
			return rule.kind, "heuristic_fallback", "low"
		}
	}
	return "other", "unknown", "low"
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

func nestedACPToolKind(metadata map[string]interface{}) string {
	rawACP, ok := metadata["acp"].(map[string]interface{})
	if !ok {
		return ""
	}
	for _, key := range []string{"tool_kind", "kind"} {
		if value, ok := rawACP[key].(string); ok {
			if kind := NormalizeOfficialToolKind(value); kind != "" {
				return kind
			}
		}
	}
	return ""
}

func effectForKind(kind string) string {
	switch kind {
	case "read", "search", "fetch", "think":
		return "read_only"
	case "edit", "delete", "move":
		return "write"
	case "execute":
		return "execute"
	case "switch_mode":
		return "control"
	default:
		return "unknown"
	}
}

func subjectForKind(kind string, metadata map[string]interface{}, path string) string {
	if strings.TrimSpace(path) != "" || StringValue(metadata, "path") != "" {
		return "filesystem"
	}
	subject := toolSubjectByKind[kind]
	if subject == "" {
		return "unknown"
	}
	return subject
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
	{kind: "think", matches: []string{"think", "reason"}},
	{kind: "switch_mode", matches: []string{"switch_mode", "set_mode"}},
}
