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
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}
