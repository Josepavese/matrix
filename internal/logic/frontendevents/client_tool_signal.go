package frontendevents

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

type ToolSignal struct {
	Content  string
	Metadata map[string]interface{}
}

func ACPClientFSToolSignal(method, path string, bytes int) ToolSignal {
	toolName, toolKind := "read_file", "read"
	if method == "fs/write_text_file" {
		toolName, toolKind = "write_file", "edit"
	}
	meta := map[string]interface{}{
		"protocol_method": method,
		"tool_name":       toolName,
		"tool_kind":       toolKind,
		"path":            filepath.Clean(path),
	}
	if bytes >= 0 {
		meta["bytes"] = bytes
	}
	return ToolSignal{Content: ToolContent(method, path), Metadata: meta}
}

func ACPClientTerminalToolSignal(command string, args []string, cwd string, exitCode *int) ToolSignal {
	meta := map[string]interface{}{
		"protocol_method":    "terminal/create",
		"tool_name":          "shell",
		"tool_kind":          "execute",
		"tool_semantic_kind": TerminalSemanticKind(command, args),
		"command":            CommandLine(command, args),
		"command_name":       command,
		"cwd":                filepath.Clean(cwd),
	}
	if exitCode != nil {
		meta["exit_code"] = *exitCode
	}
	return ToolSignal{Content: ToolContent("terminal/create", CommandLine(command, args)), Metadata: meta}
}

func ToolContent(method, subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return method
	}
	return method + " " + subject
}

func CommandLine(command string, args []string) string {
	parts := append([]string{strings.TrimSpace(command)}, args...)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " ")
}

func TerminalSemanticKind(command string, args []string) string {
	line := strings.ToLower(CommandLine(command, args))
	switch {
	case strings.Contains(line, "go test"), strings.Contains(line, "npm test"), strings.Contains(line, "pytest"), strings.Contains(line, "cargo test"):
		return "validate"
	default:
		return ""
	}
}

func SanitizedRawInput(params json.RawMessage) map[string]interface{} {
	if len(params) == 0 {
		return nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(params, &raw); err != nil {
		return nil
	}
	delete(raw, "content")
	return raw
}
