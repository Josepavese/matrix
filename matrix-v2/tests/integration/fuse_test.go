package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jose/matrix-v2/internal/logic/filesystem"
	"github.com/jose/matrix-v2/internal/providers/fusefs"
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
	defer mgr.UnmountVirtualFS()

	// Wait briefly for the OS FUSE subsystem to register the mount
	time.Sleep(200 * time.Millisecond)

	// Verify the file exists and has correct content
	filePath := filepath.Join(mountPoint, "matrix.txt")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read virtual file after mount: %v", err)
	}

	expected := "Welcome to the Matrix V2 Virtual Filesystem"
	if !strings.Contains(string(content), expected) {
		t.Errorf("Unexpected virtual file content. Got: %s", string(content))
	}

	// Verify the file is read-only
	err = os.WriteFile(filePath, []byte("write test"), 0644)
	if err == nil {
		t.Errorf("Expected write to fail on read-only FUSE mount, but it succeeded")
	}
}
