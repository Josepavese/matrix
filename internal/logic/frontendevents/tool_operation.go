package frontendevents

import (
	"encoding/json"
	"strings"
)

func ToolOperation(content string, metadata map[string]interface{}) string {
	for _, key := range []string{"operation", "command_name", "command", "cmd"} {
		if op := operationFromCommand(StringValue(metadata, key)); op != "" {
			return op
		}
	}
	if op := operationFromMap(metadata); op != "" {
		return op
	}
	if strings.TrimSpace(content) != "" {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(content), &payload); err == nil {
			return operationFromMap(payload)
		}
	}
	return ""
}

func operationFromMap(values map[string]interface{}) string {
	if values == nil {
		return ""
	}
	for _, key := range []string{"operation", "command_name", "command", "cmd"} {
		if op := operationFromCommand(stringFromAny(values[key])); op != "" {
			return op
		}
	}
	for _, value := range values {
		switch typed := value.(type) {
		case map[string]interface{}:
			if op := operationFromMap(typed); op != "" {
				return op
			}
		case []interface{}:
			for _, item := range typed {
				if nested, ok := item.(map[string]interface{}); ok {
					if op := operationFromMap(nested); op != "" {
						return op
					}
				}
			}
		}
	}
	return ""
}

func operationFromCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	op := strings.TrimSpace(fields[0])
	op = strings.TrimPrefix(op, "/usr/bin/")
	op = strings.TrimPrefix(op, "/bin/")
	if op == "" || strings.ContainsAny(op, `/\ `) {
		return ""
	}
	return op
}
