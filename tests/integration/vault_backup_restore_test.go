package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jose/matrix-v2/internal/logic/vaultsec"
	"github.com/jose/matrix-v2/internal/providers/osfs"
)

func TestVaultBackupRestoreRoundTrip(t *testing.T) {
	fs := osfs.NewFSProvider()
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "matrix-vault.db")
	backupDir := filepath.Join(dir, "backups")
	restoreSource := filepath.Join(dir, "restore-source.db")

	if err := writeFSFile(fs, vaultPath, []byte("original-vault")); err != nil {
		t.Fatalf("writeFSFile(vault): %v", err)
	}
	backupPath, err := vaultsec.CreateBackup(fs, vaultPath, backupDir, time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if err := writeFSFile(fs, restoreSource, []byte("restored-vault")); err != nil {
		t.Fatalf("writeFSFile(restoreSource): %v", err)
	}

	preRestoreBackup, err := vaultsec.RestoreBackup(fs, restoreSource, vaultPath, backupDir, time.Date(2026, 4, 15, 12, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if preRestoreBackup == "" {
		t.Fatalf("expected pre-restore backup path")
	}
	if _, err := fs.Stat(backupPath); err != nil {
		t.Fatalf("expected original backup to exist: %v", err)
	}
	restored, err := fs.ReadFile(vaultPath)
	if err != nil {
		t.Fatalf("ReadFile(vault): %v", err)
	}
	if string(restored) != "restored-vault" {
		t.Fatalf("unexpected restored content: %q", restored)
	}
}

func writeFSFile(fs *osfs.FSProvider, path string, content []byte) error {
	file, err := fs.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = file.Write(content)
	return err
}
