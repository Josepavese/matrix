//go:build !windows

package oslog

import (
	"os"
	"testing"
)

func assertPrivateLogPermissions(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("log permissions=%v, want0600", info.Mode().Perm())
	}
}
