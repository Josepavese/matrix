//go:build linux || darwin

package zedacp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStdioTransportCloseKillsProcessGroupChildren(t *testing.T) {
	tmp := t.TempDir()
	childPIDFile := filepath.Join(tmp, "child.pid")
	script := fmt.Sprintf("sleep 60 & echo $! > %q; cat >/dev/null", childPIDFile)

	transport, err := NewStdioTransport(context.Background(), "sh", nil, "-c", script)
	if err != nil {
		t.Fatalf("NewStdioTransport failed: %v", err)
	}

	childPID := waitForPIDFile(t, childPIDFile)
	if childPID <= 0 {
		t.Fatalf("invalid child pid: %d", childPID)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	waitForProcessExit(t, childPID)
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if convErr == nil {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for pid file %s", path)
	return 0
}

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processExists(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("child process %d still exists after transport close", pid)
}

func processExists(pid int) bool {
	cmd := exec.Command("kill", "-0", strconv.Itoa(pid))
	return cmd.Run() == nil
}
