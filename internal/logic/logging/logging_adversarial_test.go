package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// TestHumanSizeKeepsUnitsHonest pins the boundaries where a wrong unit turns a
// 1 KB log file into "1024 B" or, worse, a 1 MB file into "1023.99 KB".
func TestHumanSizeKeepsUnitsHonest(t *testing.T) {
	cases := map[int64]string{
		0:                  "0 B",
		1023:               "1023 B",
		1024:               "1.00 KB",
		1024 * 1024:        "1.00 MB",
		1024 * 1024 * 1024: "1.00 GB",
	}
	for size, want := range cases {
		if got := humanSize(size); got != want {
			t.Fatalf("humanSize(%d) = %q, want %q", size, got, want)
		}
	}
}

// TestFileSizeRejectsAMissingFile keeps a doctor report from claiming a healthy
// zero-byte log file when the file simply is not there.
func TestFileSizeRejectsAMissingFile(t *testing.T) {
	fs := osfs.NewFSProvider()
	if _, err := fileSize(fs, filepath.Join(t.TempDir(), "absent.log")); err == nil {
		t.Fatal("a missing file must be an error, not a zero size")
	}
}

// TestFindBackupFilesOnAnEmptyDirectory is the degraded case: no backups is a
// normal state and must not be reported as an error.
func TestFindBackupFilesOnAnEmptyDirectory(t *testing.T) {
	matches, err := findBackupFiles(filepath.Join(t.TempDir(), "matrix.log"))
	if err != nil {
		t.Fatalf("a missing directory must not be an error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no backups, got %v", matches)
	}
}

// TestBuildDoctorReportSeparatesMissingFromOversized is the doctor's decision
// logic: a log file that was never created is normal, while one that outgrew its
// rotation budget means rotation is not working and must be warned about.
func TestBuildDoctorReportSeparatesMissingFromOversized(t *testing.T) {
	fs := osfs.NewFSProvider()
	dir := t.TempDir()

	missing := filepath.Join(dir, "absent.log")
	report, err := BuildDoctorReport(fs, Config{FilePath: missing, MaxBytes: 1024, MaxBackups: 2}, nil)
	if err != nil {
		t.Fatalf("a missing log file is a normal state, not an error: %v", err)
	}
	if report["exists"] != false {
		t.Fatalf("a missing file must report exists=false, got %v", report["exists"])
	}
	if warnings, _ := report["warnings"].([]string); len(warnings) != 0 {
		t.Fatalf("a missing file must not produce warnings: %v", warnings)
	}

	oversized := filepath.Join(dir, "matrix.log")
	if err := os.WriteFile(oversized, bytes.Repeat([]byte("x"), 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err = BuildDoctorReport(fs, Config{FilePath: oversized, MaxBytes: 1024, MaxBackups: 2}, nil)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if report["exists"] != true {
		t.Fatal("an existing log file must be reported")
	}
	if report["size_bytes"] != int64(4096) {
		t.Fatalf("size_bytes = %v, want 4096", report["size_bytes"])
	}
	if report["size_human"] != "4.00 KB" {
		t.Fatalf("size_human = %v, want 4.00 KB", report["size_human"])
	}
	warnings, _ := report["warnings"].([]string)
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning, "max_bytes") {
			found = true
		}
	}
	if !found {
		t.Fatalf("an oversized log file must warn about rotation: %v", warnings)
	}
}
