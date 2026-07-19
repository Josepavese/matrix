package agentcfg

import (
	"fmt"
	"sort"
	"strings"
)

// ParseHeaders validates repeatable Name=Value endpoint header arguments.
func ParseHeaders(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(values))
	for _, value := range values {
		name, headerValue, ok := strings.Cut(value, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid endpoint header %q; expected Name=Value", value)
		}
		out[name] = strings.TrimSpace(headerValue)
	}
	return out, nil
}

// HeaderNames returns deterministic non-secret header names for display.
func HeaderNames(headers map[string]string) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
