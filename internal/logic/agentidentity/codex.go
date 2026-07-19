package agentidentity

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	CanonicalCodexAgentID  = "codex"
	CodexRegistryID        = "codex-acp"
	CanonicalCodexPackage  = "@agentclientprotocol/codex-acp"
	DeprecatedCodexPackage = "@zed-industries/codex-acp"
)

func CanonicalRegistryAlias(agentID string) string {
	if agentID == CanonicalCodexAgentID {
		return CodexRegistryID
	}
	return ""
}

// ValidatePublicAgentID keeps the ACP registry identifier out of Matrix's
// public routing and persistence contracts. It is not a compatibility alias.
func ValidatePublicAgentID(agentID string) error {
	if agentID == CodexRegistryID {
		return fmt.Errorf("%q is an ACP registry/provider identifier, not a Matrix agent ID; use %q", agentID, CanonicalCodexAgentID)
	}
	return nil
}

func PublicAgentIDHint(agentID string) string {
	if err := ValidatePublicAgentID(agentID); err != nil {
		return "; " + err.Error()
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

// ValidateRuntimeDefinition rejects retired Codex providers before a daemon or
// command can launch them. Explicit reinstall/uninstall paths remain available
// for one-way migration.
func ValidateRuntimeDefinition(agentID string, command string, args []string) error {
	if err := ValidatePublicAgentID(agentID); err != nil {
		return fmt.Errorf("ZERO-LEGACY policy violation: %w", err)
	}
	if DeprecatedCodexSource(command, args, "") != "" {
		return fmt.Errorf("ZERO-LEGACY policy violation: agent %q uses retired provider %s; run `matrix install %s`", agentID, DeprecatedCodexPackage, CanonicalCodexAgentID)
	}
	return nil
}
