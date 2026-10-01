//go:build linux

package childidentity

import (
	"fmt"
	"os"
)

// readIdentity reads /proc/<pid>/cwd and /proc/<pid>/cmdline for a process
// Matrix started.
func readIdentity(pid int) (Identity, error) {
	if pid <= 0 {
		return Identity{}, fmt.Errorf("no child process to inspect")
	}
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return Identity{}, fmt.Errorf("read child cwd: %w", err)
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return Identity{}, fmt.Errorf("read child argv: %w", err)
	}
	return Identity{PID: pid, Cwd: cwd, Argv: splitNULArgv(raw)}, nil
}
