package runtimecheck

import "strings"

// AppendReadinessWarnings adds runtime warnings that are relevant for readiness.
func AppendReadinessWarnings(warnings *[]string, raw any, expectRuntimeUp bool) {
	appendFilteredWarning := func(text string) {
		if !expectRuntimeUp && isOptionalRuntimeDownWarning(text) {
			return
		}
		*warnings = append(*warnings, text)
	}

	switch values := raw.(type) {
	case []any:
		for _, value := range values {
			if text, ok := value.(string); ok {
				appendFilteredWarning(text)
			}
		}
	case []string:
		for _, text := range values {
			appendFilteredWarning(text)
		}
	}
}

func isOptionalRuntimeDownWarning(text string) bool {
	return strings.HasPrefix(text, "jsonrpc daemon is not reachable") ||
		strings.HasPrefix(text, "matrix http ingress is not reachable") ||
		strings.HasPrefix(text, "a2a http server is not reachable")
}
