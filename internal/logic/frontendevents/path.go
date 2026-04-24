package frontendevents

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

var absolutePathPattern = regexp.MustCompile(`(/[\w./@+=,_-]+)`)

func FirstPath(content string, metadata map[string]interface{}) string {
	for _, key := range []string{"path", "file", "filename", "target", "cwd"} {
		if value := StringValue(metadata, key); strings.HasPrefix(value, "/") {
			return filepath.Clean(value)
		}
	}
	if path := firstPathFromMap(metadata); path != "" {
		return path
	}
	if strings.TrimSpace(content) != "" {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(content), &payload); err == nil {
			if path := firstPathFromMap(payload); path != "" {
				return path
			}
		}
	}
	match := absolutePathPattern.FindString(content)
	if match == "" {
		return ""
	}
	return filepath.Clean(match)
}

func firstPathFromMap(values map[string]interface{}) string {
	for _, key := range []string{"path", "file", "filename", "target"} {
		if value := stringFromAny(values[key]); strings.HasPrefix(value, "/") {
			return filepath.Clean(value)
		}
	}
	for key, value := range values {
		if path := firstPathFromValue(key, value); path != "" {
			return path
		}
	}
	return ""
}

func firstPathFromValue(key string, value interface{}) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		return firstPathFromMap(typed)
	case []interface{}:
		return firstPathFromSlice(typed)
	case string:
		return pathFromCommandSource(key, typed)
	default:
		return ""
	}
}

func firstPathFromSlice(values []interface{}) string {
	for _, item := range values {
		nested, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if path := firstPathFromMap(nested); path != "" {
			return path
		}
	}
	return ""
}

func pathFromCommandSource(key, value string) string {
	if !isCommandPathSource(key) {
		return ""
	}
	match := absolutePathPattern.FindString(value)
	if match == "" {
		return ""
	}
	return filepath.Clean(match)
}

func isCommandPathSource(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "command", "cmd", "arguments", "args":
		return true
	default:
		return false
	}
}

func stringFromAny(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}
