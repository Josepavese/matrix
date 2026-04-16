package runnotifier

import "strings"

func inferToolName(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	for _, sep := range []string{" ", "\n", ":", "("} {
		if idx := strings.Index(content, sep); idx > 0 {
			return strings.TrimSpace(content[:idx])
		}
	}
	if len(content) > 64 {
		return content[:64]
	}
	return content
}
