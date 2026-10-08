package oscapacity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeObservationUsesActualWorkspaceVolume(t *testing.T) {
	dir := t.TempDir()
	first, err := New().Observe(dir)
	if err != nil || !first.Available || first.DiskTotalBytes == nil || *first.DiskTotalBytes == 0 || first.VolumeID == "" {
		t.Fatalf("native capacity unavailable: %+v %v", first, err)
	}
	if first.DiskFreeBytes == nil || *first.DiskFreeBytes > *first.DiskTotalBytes || first.LogicalCPUs < 1 || first.MeasuredAt.IsZero() {
		t.Fatalf("invalid observation: %+v", first)
	}
	child := filepath.Join(dir, "nested")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	second, err := New().Observe(child)
	if err != nil || second.VolumeID != first.VolumeID || second.DiskTotalBytes == nil || *second.DiskTotalBytes != *first.DiskTotalBytes {
		t.Fatalf("two paths on the same volume disagreed: %+v %v", second, err)
	}
	entries, err := os.ReadDir(child)
	if err != nil || len(entries) != 0 {
		t.Fatal("observer wrote into the workspace")
	}
}

func TestUnavailablePathsNeverSubstituteRootFilesystem(t *testing.T) {
	file := filepath.Join(t.TempDir(), "regular")
	if err := os.WriteFile(file, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", file, file + "-missing"} {
		result, err := New().Observe(path)
		if err == nil || result.Available || result.Reason == "" || result.VolumeID != "" {
			t.Fatalf("unknown workspace falsely observed: %+v %v", result, err)
		}
	}
}
