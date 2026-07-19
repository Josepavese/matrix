package vaultsec

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/providers/osfs"
)

type staticEncryptionInspector struct {
	encrypted int
	plaintext int
}

func (s staticEncryptionInspector) InspectRawEncryption() (int, int, error) {
	return s.encrypted, s.plaintext, nil
}

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

func TestBuildReportUsesProvidedRawEncryptionInspector(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))
	path := filepath.Join(dir, "matrix-vault.db")
	if err := os.WriteFile(path, []byte("provider-owned"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := BuildReport(osfs.NewFSProvider(), path, staticEncryptionInspector{encrypted: 4, plaintext: 0})
	if err != nil {
		t.Fatal(err)
	}
	encryption, ok := report["encryption"].(map[string]any)
	if !ok {
		t.Fatalf("missing encryption report: %+v", report)
	}
	if encryption["encrypted_keys"] != 4 || encryption["plaintext_keys"] != 0 {
		t.Fatalf("unexpected encryption report: %+v", encryption)
	}
	warnings, ok := report["warnings"].([]string)
	if !ok || len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}
