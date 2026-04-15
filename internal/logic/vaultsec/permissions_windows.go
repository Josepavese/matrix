//go:build windows

package vaultsec

import "os"

func ApplySecurePermissions(path string) error {
	return nil
}

func permissionsSupported() bool {
	return false
}

func permissionsModel() string {
	return "windows-acl"
}

func securePermissions(mode os.FileMode) bool {
	return true
}

func permissionsString(mode os.FileMode) string {
	return mode.String()
}
