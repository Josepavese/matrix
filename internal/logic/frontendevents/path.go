package frontendevents

import (
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
	match := absolutePathPattern.FindString(content)
	if match == "" {
		return ""
	}
	return filepath.Clean(match)
}
