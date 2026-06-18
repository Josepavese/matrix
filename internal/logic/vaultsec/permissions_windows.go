//go:build windows

package vaultsec

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

func ApplySecurePermissions(path string) error {
	current, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to resolve current user for ACL hardening: %w", err)
	}
	cmd := exec.Command(
		"icacls",
		path,
		"/inheritance:r",
		"/remove:g", "*S-1-1-0", "*S-1-5-11", "*S-1-5-32-545",
		"/grant:r", current.Username+":(F)",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to apply Windows ACLs to %s: %w: %s", path, err, string(output))
	}
	return nil
}

func permissionsSupported() bool {
	return true
}

func permissionsModel() string {
	return "windows-acl"
}

func securePermissions(mode os.FileMode) bool {
	return mode.IsRegular()
}

func securePathPermissions(path string, mode os.FileMode) bool {
	if !mode.IsRegular() {
		return false
	}
	output, err := exec.Command("icacls", path).CombinedOutput()
	if err != nil {
		return false
	}
	acl := strings.ToLower(string(output))
	for _, principal := range []string{"everyone", "authenticated users", "builtin\\users", "s-1-1-0", "s-1-5-11", "s-1-5-32-545"} {
		if strings.Contains(acl, strings.ToLower(principal)) {
			return false
		}
	}
	return true
}

func permissionsString(mode os.FileMode) string {
	return mode.String()
}
