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
		switch typed := value.(type) {
		case map[string]interface{}:
			if path := firstPathFromMap(typed); path != "" {
				return path
			}
		case []interface{}:
			for _, item := range typed {
				if nested, ok := item.(map[string]interface{}); ok {
					if path := firstPathFromMap(nested); path != "" {
						return path
					}
				}
			}
		case string:
			if isCommandPathSource(key) {
				match := absolutePathPattern.FindString(typed)
				if match != "" {
					return filepath.Clean(match)
				}
			}
		}
	}
	return ""
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
