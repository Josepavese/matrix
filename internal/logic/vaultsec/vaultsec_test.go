package vaultsec

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/providers/osfs"
)

func TestCreateBackup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "matrix-vault.db")
	if err := os.WriteFile(src, []byte("vault-bytes"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	fs := osfs.NewFSProvider()
	backupDir := filepath.Join(dir, "backups")
	path, err := CreateBackup(fs, src, backupDir, time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if filepath.Base(path) != "matrix-vault-20260402-120000.db" {
		t.Fatalf("unexpected backup name: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != "vault-bytes" {
		t.Fatalf("unexpected backup content: %q", string(data))
	}
}
