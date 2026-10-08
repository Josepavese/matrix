package integration

import (
	"github.com/Josepavese/matrix/internal/logic/filesystem"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/semanticfs"
	"github.com/Josepavese/matrix/internal/providers/fusefs"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestFUSE_MountAndRead(t *testing.T) {
	driver := os.Getenv("MATRIX_REAL_FUSE_DRIVER")
	if driver == "" {
		t.Skip("set MATRIX_REAL_FUSE_DRIVER for a native FUSE/WinFsp mount proof")
	}
	mountPoint := filepath.Join(t.TempDir(), "matrix-mnt")
	view := fstest.MapFS{"runs/proof/status.json": &fstest.MapFile{Data: []byte(`{"status":"running"}`), Mode: 0400}}
	provider := fusefs.NewProvider().WithView(view).WithDriverPath(driver)
	manager := filesystem.NewManager(provider)
	if err := manager.MountVirtualFS(mountPoint); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := manager.UnmountVirtualFS(); err != nil {
			t.Error(err)
		}
	}()
	file := filepath.Join(mountPoint, "runs", "proof", "status.json")
	content, err := os.ReadFile(file)
	if err != nil || string(content) != `{"status":"running"}` {
		t.Fatal("native semantic read failed", err)
	}
	if err := os.WriteFile(file, []byte("modified"), 0600); err == nil {
		t.Fatal("read-only mount accepted a write")
	}
	if err := manager.UnmountVirtualFS(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(mountPoint)
	if err != nil || len(entries) != 0 {
		t.Fatal("native unmount was not observed", err)
	}
}

func TestSmokeFUSELiveSemanticStatusChanges(t *testing.T) {
	driver := os.Getenv("MATRIX_REAL_FUSE_DRIVER")
	if driver == "" {
		t.Skip("set MATRIX_REAL_FUSE_DRIVER for native live projection")
	}
	store := memstore.New()
	runs := runtrace.NewStore(store)
	run, _, err := runs.Start(runtrace.Run{ID: "mount-live-proof"})
	if err != nil {
		t.Fatal(err)
	}
	view := semanticfs.New(semanticfs.Source{Storage: store})
	name, _ := semanticfs.DirectoryName(run.ID)
	leaf := "runs/" + name + "/status.json"
	mount := t.TempDir()
	provider := fusefs.NewProvider().WithView(view).WithDriverPath(driver)
	if err := provider.Mount(mount); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := provider.Unmount(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := os.ReadFile(filepath.Join(mount, filepath.FromSlash(leaf))); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Complete(run.ID, "PRIVATE_SUMMARY", "end_turn"); err != nil {
		t.Fatal(err)
	}
	want, err := fs.ReadFile(view, leaf)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(mount, filepath.FromSlash(leaf)))
		if err == nil && string(data) == string(want) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("mounted status did not refresh or size remained stale")
}
