package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestSmokePALNativeCapacityCLI(t *testing.T) {
	bin := os.Getenv("MATRIX_PAL_BINARY")
	if bin == "" {
		t.Skip("set MATRIX_PAL_BINARY to the native build")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	workspace := filepath.Join(t.TempDir(), "workspace with spaces")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, bin, "capacity", workspace)
	cmd.Env = append(os.Environ(), "MATRIX_HOME="+t.TempDir())
	output, err := cmd.Output()
	if err != nil {
		t.Fatal("native CLI failed", err)
	}
	var observed middleware.CapacitySnapshot
	if err := json.Unmarshal(output, &observed); err != nil {
		t.Fatal(err)
	}
	if !observed.Available || observed.DiskFreeBytes == nil || observed.LogicalCPUs < 1 || observed.VolumeID == "" {
		t.Fatal("native CLI fabricated/missed capacity")
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Fatal("capacity wrote into workspace")
	}
	t.Logf("native %s/%s capacity observed: disk/volume/CPU; workspace unchanged", runtime.GOOS, runtime.GOARCH)
}
