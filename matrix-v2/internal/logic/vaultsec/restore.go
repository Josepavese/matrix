package vaultsec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

func RestoreBackup(fs middleware.FS, sourcePath, targetPath, backupDir string, now time.Time) (string, error) {
	absSource, absTarget, err := validateRestorePaths(fs, sourcePath, targetPath)
	if err != nil {
		return "", err
	}
	preRestoreBackup, err := backupIfPresent(fs, absTarget, backupDir, now)
	if err != nil {
		return "", err
	}
	if err := replaceTarget(fs, absSource, absTarget); err != nil {
		return "", err
	}
	return preRestoreBackup, nil
}

func validateRestorePaths(fs middleware.FS, sourcePath, targetPath string) (string, string, error) {
	if sourcePath == "" || targetPath == "" {
		return "", "", fmt.Errorf("source and target paths are required")
	}
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", "", err
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", "", err
	}
	if absSource == absTarget {
		return "", "", fmt.Errorf("restore source and target must be different")
	}
	srcInfo, err := fs.Stat(absSource)
	if err != nil {
		return "", "", err
	}
	if !srcInfo.Mode().IsRegular() {
		return "", "", fmt.Errorf("restore source must be a regular file")
	}
	return absSource, absTarget, nil
}

func backupIfPresent(fs middleware.FS, targetPath, backupDir string, now time.Time) (string, error) {
	if _, err := fs.Stat(targetPath); err == nil {
		return CreateBackup(fs, targetPath, backupDir, now)
	}
	return "", nil
}

func replaceTarget(fs middleware.FS, sourcePath, targetPath string) error {
	tmpPath := targetPath + ".restore.tmp"
	if err := copyFile(fs, sourcePath, tmpPath); err != nil {
		_ = fs.Remove(tmpPath)
		return err
	}
	if err := ApplySecurePermissions(tmpPath); err != nil {
		_ = fs.Remove(tmpPath)
		return err
	}
	if err := fs.Rename(tmpPath, targetPath); err != nil {
		_ = fs.Remove(tmpPath)
		return err
	}
	return nil
}

func copyFile(fs middleware.FS, sourcePath, targetPath string) error {
	src, err := fs.Open(sourcePath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := fs.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	_, err = io.Copy(dst, src)
	return err
}
