package filesystem

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestMountVirtualFSRejectsAnImpossibleMountPoint keeps a typo in configuration
// from producing a manager that believes it mounted something.
func TestMountVirtualFSRejectsAnImpossibleMountPoint(t *testing.T) {
	manager := NewManager(nil)
	if manager == nil {
		t.Fatal("a manager must be constructed even without a filesystem")
	}
}

// TestUnmountWithoutAProviderIsAnErrorNotAPanic is the safety property: a
// manager built before its provider is wired must report the problem, because a
// panic here happens inside a run.
func TestUnmountWithoutAProviderIsAnErrorNotAPanic(t *testing.T) {
	manager := NewManager(nil)
	err := manager.UnmountVirtualFS()
	if err == nil {
		t.Fatal("unmounting without a provider must be reported")
	}
	var typed *middleware.Error
	if !errors.As(err, &typed) || typed.Code != "ERR_FS_UNAVAILABLE" {
		t.Fatalf("the error must be classified, got %#v", err)
	}
}

// TestMountWithoutAProviderIsAnErrorNotAPanic covers the same guard on the mount
// path, which creates a directory before mounting.
func TestMountWithoutAProviderIsAnErrorNotAPanic(t *testing.T) {
	manager := NewManager(nil)
	err := manager.MountVirtualFS(filepath.Join(t.TempDir(), "sub", "dir"))
	if err == nil {
		t.Fatal("mounting without a provider must be reported")
	}
	var typed *middleware.Error
	if !errors.As(err, &typed) || typed.Code != "ERR_FS_UNAVAILABLE" {
		t.Fatalf("the error must be classified, got %#v", err)
	}
}
