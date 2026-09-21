package runconfig

import (
	"errors"
	"path/filepath"
	"strings"
)

// NormalizeAdditionalDirectories validates, trims, and de-duplicates run directories.
func NormalizeAdditionalDirectories(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			return nil, errors.New("additional_directories entries must be absolute paths")
		}
		// Normalise before de-duplicating: "/a", "/a/" and "/a/./" are the same
		// directory and must not reach the agent as three entries.
		value = filepath.Clean(value)
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}
