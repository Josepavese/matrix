package vaultsec

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/providers/osfs"
)

func TestRestoreBackup(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	target := filepath.Join(dir, "target.db")
	if err := os.WriteFile(source, []byte("new-data"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte("old-data"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	fs := osfs.NewFSProvider()
	backup, err := RestoreBackup(fs, source, target, filepath.Join(dir, "backups"), time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if backup == "" {
		t.Fatalf("expected pre-restore backup path")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "new-data" {
		t.Fatalf("unexpected target data: %q", string(data))
	}
}
