package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/filesystem"
	"github.com/Josepavese/matrix/internal/providers/fusefs"
)

func TestFUSE_MountAndRead(t *testing.T) {
	tempDir := t.TempDir()
	mountPoint := filepath.Join(tempDir, "matrix-mnt")

	provider := fusefs.NewProvider()
	mgr := filesystem.NewManager(provider)

	// Mount the FUSE synthetic filesystem
	if err := mgr.MountVirtualFS(mountPoint); err != nil {
		t.Fatalf("Failed to mount virtual FS: %v", err)
	}
	defer func() {
		if err := mgr.UnmountVirtualFS(); err != nil {
			t.Fatalf("UnmountVirtualFS failed: %v", err)
		}
	}()

	// The kernel registers the mount asynchronously: wait for the observable
	// fact — the file is readable — instead of sleeping a fixed 200ms and hoping
	// the mount won the race. Five seconds is 250x the 20ms poll interval.
	filePath := filepath.Join(mountPoint, "matrix.txt")
	var (
		content []byte
		err     error
	)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, err = os.ReadFile(filePath)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Failed to read virtual file after mount: %v", err)
	}

	expected := "Welcome to the Matrix Virtual Filesystem"
	if !strings.Contains(string(content), expected) {
		t.Errorf("Unexpected virtual file content. Got: %s", string(content))
	}

	// Verify the file is read-only
	err = os.WriteFile(filePath, []byte("write test"), 0644)
	if err == nil {
		t.Errorf("Expected write to fail on read-only FUSE mount, but it succeeded")
	}
}
