package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jose/matrix-v2/internal/middleware"
)

func BuildDoctorReport(fs middleware.FS, cfg Config, warnings []string) (map[string]any, error) {
	backups := []string{}
	report := map[string]any{
		"sink":        cfg.Sink,
		"format":      cfg.Format,
		"file_path":   cfg.FilePath,
		"max_bytes":   cfg.MaxBytes,
		"max_backups": cfg.MaxBackups,
		"stderr":      cfg.StdErr,
		"acp_wire":    cfg.ACPWire,
		"exists":      false,
		"size_bytes":  int64(0),
		"size_human":  "0 B",
		"backups":     backups,
		"warnings":    warnings,
	}

	size, err := fileSize(fs, cfg.FilePath)
	if err == nil {
		report["exists"] = true
		report["size_bytes"] = size
		report["size_human"] = humanSize(size)
		if size > cfg.MaxBytes {
			warnings = append(warnings, "active log file exceeds configured max_bytes")
		}
	} else if !os.IsNotExist(err) {
		warnings = append(warnings, "failed to stat active log file: "+err.Error())
	}

	backups, err = findBackupFiles(cfg.FilePath)
	if err != nil {
		warnings = append(warnings, "failed to enumerate backup logs: "+err.Error())
	} else {
		if backups == nil {
			backups = []string{}
		}
		report["backups"] = backups
		if len(backups) > cfg.MaxBackups {
			warnings = append(warnings, "backup count exceeds configured max_backups")
		}
	}
	report["warnings"] = warnings

	return report, nil
}

func findBackupFiles(path string) ([]string, error) {
	matches, err := filepath.Glob(path + ".*")
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func fileSize(fs middleware.FS, path string) (int64, error) {
	info, err := fs.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func humanSize(size int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case size >= gb:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(gb))
	case size >= mb:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(mb))
	case size >= kb:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(kb))
	default:
		return fmt.Sprintf("%d B", size)
	}
}
