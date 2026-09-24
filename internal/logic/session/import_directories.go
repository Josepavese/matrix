package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// verifiedRemoteDirectories keeps only canonical, locally authorized paths
// announced by a provider. A path outside the imported workspace requires an
// exact explicit directory in the import request.
func verifiedRemoteDirectories(reported, authorized []string, workspace string) ([]string, error) {
	allowed := make(map[string]bool, len(authorized))
	for _, path := range authorized {
		canonical, err := canonicalImportDirectory(path)
		if err != nil {
			return nil, fmt.Errorf("additional_directories: %w", err)
		}
		allowed[canonical] = true
	}
	seen := make(map[string]bool, len(reported))
	out := make([]string, 0, len(reported))
	for _, path := range reported {
		canonical, err := canonicalImportDirectory(path)
		if err != nil {
			return nil, fmt.Errorf("provider additional directory: %w", err)
		}
		if !pathWithinWorkspace(canonical, workspace) && !allowed[canonical] {
			return nil, fmt.Errorf("workspace_mismatch: provider announced unauthorized additional directory %s", canonical)
		}
		if !seen[canonical] {
			out = append(out, canonical)
			seen[canonical] = true
		}
	}
	return out, nil
}

func canonicalImportDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute: %q", path)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", canonical)
	}
	return filepath.Clean(canonical), nil
}

func pathWithinWorkspace(path, workspace string) bool {
	rel, err := filepath.Rel(workspace, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
