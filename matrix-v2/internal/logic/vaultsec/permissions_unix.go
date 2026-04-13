//go:build !windows

package vaultsec

import "os"

func ApplySecurePermissions(path string) error {
	return os.Chmod(path, 0o600)
}

func permissionsSupported() bool {
	return true
}

func permissionsModel() string {
	return "posix"
}

func securePermissions(mode os.FileMode) bool {
	return mode.Perm() == 0o600
}

func permissionsString(mode os.FileMode) string {
	return mode.Perm().String()
}
