package vaultsec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"

	bbolt "go.etcd.io/bbolt"
)

// BuildReport generates a vault security report.
func BuildReport(fs middleware.FS, path string) (map[string]any, error) {
	_, keyStatus, err := ResolveMasterKey(fs)
	if err != nil {
		return nil, err
	}
	report := newReport(path, keyStatus)

	info, err := fs.Stat(path)
	if err != nil {
		return handleMissingVault(report, keyStatus, err)
	}

	report["warnings"] = collectWarnings(fs, path, info, keyStatus, report)
	return report, nil
}

func newReport(path string, keyStatus KeyStatus) map[string]any {
	return map[string]any{
		"path":               path,
		"exists":             false,
		"size_bytes":         int64(0),
		"permissions":        "",
		"permissions_secure": false,
		"permissions_model":  permissionsModel(),
		"encryption":         encryptionReport(keyStatus, encryptionMeta{}),
		"warnings":           []string{},
	}
}

func handleMissingVault(report map[string]any, keyStatus KeyStatus, err error) (map[string]any, error) {
	if !os.IsNotExist(err) {
		return nil, err
	}
	report["warnings"] = baseWarnings(keyStatus)
	return report, nil
}

func collectWarnings(_ middleware.FS, path string, info os.FileInfo, keyStatus KeyStatus, report map[string]any) []string {
	warnings := baseWarnings(keyStatus)
	report["exists"] = true
	report["size_bytes"] = info.Size()
	report["permissions"] = permissionsString(info.Mode())
	report["permissions_secure"] = securePermissions(info.Mode())
	if permissionsSupported() && !securePermissions(info.Mode()) {
		warnings = append(warnings, "vault file permissions are broader than recommended")
	}

	meta, err := inspectEncryption(path)
	if err != nil {
		return append(warnings, "failed to inspect raw vault encryption state: "+err.Error())
	}
	report["encryption"] = encryptionReport(keyStatus, meta)
	if keyStatus.Configured && meta.plaintextKeys > 0 {
		warnings = append(warnings, "vault contains plaintext entries; run `matrix vault seal` to rewrite them encrypted")
	}
	return warnings
}

func baseWarnings(keyStatus KeyStatus) []string {
	if keyStatus.Configured {
		return []string{}
	}
	return []string{"vault encryption master key is not configured"}
}

func encryptionReport(keyStatus KeyStatus, meta encryptionMeta) map[string]any {
	return map[string]any{
		"configured":     keyStatus.Configured,
		"source":         keyStatus.Source,
		"algorithm":      keyStatus.Algorithm,
		"encrypted_keys": meta.encryptedKeys,
		"plaintext_keys": meta.plaintextKeys,
		"mixed_mode":     meta.encryptedKeys > 0 && meta.plaintextKeys > 0,
	}
}

// CreateBackup copies the vault file to a backup directory.
func CreateBackup(fs middleware.FS, path, destDir string, now time.Time) (string, error) {
	if destDir == "" {
		destDir = filepath.Join(".", "backups")
	}
	if err := fs.MkdirAll(destDir, 0o700); err != nil {
		return "", err
	}

	src, err := fs.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = src.Close() }()

	filename := fmt.Sprintf("matrix-vault-%s.db", now.UTC().Format("20060102-150405"))
	destPath := filepath.Join(destDir, filename)
	dst, err := fs.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = fs.Remove(destPath)
		return "", err
	}
	if err := dst.Close(); err != nil {
		_ = fs.Remove(destPath)
		return "", err
	}
	if err := ApplySecurePermissions(destPath); err != nil {
		return "", err
	}
	return destPath, nil
}

type encryptionMeta struct {
	encryptedKeys int
	plaintextKeys int
}

func inspectEncryption(path string) (encryptionMeta, error) {
	db, err := bbolt.Open(path, 0o600, &bbolt.Options{ReadOnly: true, Timeout: 1 * time.Second})
	if err != nil {
		return encryptionMeta{}, err
	}
	defer func() { _ = db.Close() }()

	meta := encryptionMeta{}
	err = db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("matrix_vault"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			if IsEncryptedValue(v) {
				meta.encryptedKeys++
				return nil
			}
			meta.plaintextKeys++
			return nil
		})
	})
	return meta, err
}
