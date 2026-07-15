package agentidentity

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	CanonicalCodexPackage  = "@agentclientprotocol/codex-acp"
	DeprecatedCodexPackage = "@zed-industries/codex-acp"
)

func CanonicalRegistryAlias(agentID string) string {
	if agentID == "codex" {
		return "codex-acp"
	}
	return ""
}

func ValidateProviderPackage(pkg string) error {
	if strings.HasPrefix(pkg, DeprecatedCodexPackage) {
		return fmt.Errorf("provider package %s is deprecated; registry must use %s", pkg, CanonicalCodexPackage)
	}
	return nil
}

func IsCanonicalCodexPackage(pkg string) bool {
	return strings.HasPrefix(pkg, CanonicalCodexPackage+"@")
}

func DeprecatedCodexSource(command string, args []string, repository string) string {
	sources := []string{command, repository, strings.Join(args, " ")}
	if resolved, err := filepath.EvalSymlinks(command); err == nil {
		sources = append(sources, resolved)
	}
	for _, source := range sources {
		if strings.Contains(strings.ToLower(filepath.ToSlash(source)), strings.ToLower(DeprecatedCodexPackage)) {
			return source
		}
	}
	return ""
}
