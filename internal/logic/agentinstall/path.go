package agentinstall

import (
	"fmt"
	"path/filepath"
	"strings"
)

func AgentDir(baseDir, agentID string) (string, error) {
	agentID, err := safePathToken("agent id", agentID)
	if err != nil {
		return "", err
	}
	target := filepath.Join(baseDir, agentID)
	rel, err := filepath.Rel(baseDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("agent path escapes install directory")
	}
	return target, nil
}

func TempArchive(tempDir, agentID, version, archiveURL string) (string, error) {
	tempDir = strings.TrimSpace(tempDir)
	if tempDir == "" {
		return "", fmt.Errorf("temporary directory is required")
	}
	agentID, err := safePathToken("agent id", agentID)
	if err != nil {
		return "", err
	}
	version, err = safePathToken("agent version", version)
	if err != nil {
		return "", err
	}
	target := filepath.Join(tempDir, fmt.Sprintf("matrix-agent-%s-%s%s", agentID, version, archiveExt(archiveURL)))
	rel, err := filepath.Rel(tempDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("temporary archive path escapes temp directory")
	}
	return target, nil
}

func safePathToken(label, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) || strings.ContainsAny(value, `/\`) || value == "." || value == ".." {
		return "", fmt.Errorf("invalid %s %q", label, value)
	}
	return value, nil
}

func archiveExt(url string) string {
	lower := strings.ToLower(url)
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tgz", ".tbz2", ".txz", ".zip"} {
		if strings.HasSuffix(lower, ext) {
			return ext
		}
	}
	return filepath.Ext(url)
}
