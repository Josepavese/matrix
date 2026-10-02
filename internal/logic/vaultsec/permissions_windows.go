//go:build windows

package vaultsec

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/childenv"
)

// icaclsCommand is the system tool that reads and writes Windows ACLs, started
// with the shared child allowlist instead of the daemon's environment: hardening
// a vault file needs the ACL tool, not the operator's keys, and a child of Matrix
// is never handed what it does not need.
func icaclsCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("icacls", args...)
	cmd.Env = childenv.Environment()
	return cmd
}

func ApplySecurePermissions(path string) error {
	current, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to resolve current user for ACL hardening: %w", err)
	}
	cmd := icaclsCommand(
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
	output, err := icaclsCommand(path).CombinedOutput()
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
