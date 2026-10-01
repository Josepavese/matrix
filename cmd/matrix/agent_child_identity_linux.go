//go:build linux

package main

import (
	"fmt"
	"os"
)

// readChildIdentity reads /proc/<pid>/cwd and /proc/<pid>/cmdline for a process
// Matrix started.
func readChildIdentity(pid int) (childIdentity, error) {
	if pid <= 0 {
		return childIdentity{}, fmt.Errorf("no child process to inspect")
	}
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return childIdentity{}, fmt.Errorf("read child cwd: %w", err)
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return childIdentity{}, fmt.Errorf("read child argv: %w", err)
	}
	return childIdentity{PID: pid, Cwd: cwd, Argv: splitNULArgv(raw)}, nil
}
