package frontendevents

import (
	"encoding/json"
	"strings"
)

func ToolOperation(content string, metadata map[string]interface{}) string {
	for _, key := range []string{"operation", "command_name", "command", "cmd"} {
		if op := OperationFromCommand(StringValue(metadata, key)); op != "" {
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

// OperationFromStructured derives an operation identity from structured field
// values, such as an ACP permission subject or a tool call's raw input. Only
// declared operation/command keys are read, including inside nested structured
// values; free-form prose is never searched, so a sentence cannot invent one.
func OperationFromStructured(values map[string]interface{}) string {
	return operationFromMap(values)
}

func operationFromMap(values map[string]interface{}) string {
	if values == nil {
		return ""
	}
	for _, key := range []string{"operation", "command_name", "command", "cmd"} {
		if op := OperationFromCommand(stringFromAny(values[key])); op != "" {
			return op
		}
	}
	for _, value := range values {
		if op := operationFromValue(value); op != "" {
			return op
		}
	}
	return ""
}

func operationFromValue(value interface{}) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		return operationFromMap(typed)
	case []interface{}:
		return operationFromSlice(typed)
	default:
		return ""
	}
}

func operationFromSlice(values []interface{}) string {
	for _, item := range values {
		nested, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if op := operationFromMap(nested); op != "" {
			return op
		}
	}
	return ""
}

// OperationFromCommand derives the operation identity a declared command names:
// the executable, with the directory prefixes Matrix knows how to drop. A value
// that is a path or contains a separator is rejected rather than reported as an
// operation.
func OperationFromCommand(command string) string {
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
