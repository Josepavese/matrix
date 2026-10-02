//go:build linux

package childidentity

import (
	"fmt"
	"os"
)

// readIdentity reads /proc/<pid>/cwd, /proc/<pid>/cmdline and the parent and
// start time from /proc/<pid>/stat for a process Matrix started.
//
// The start time is read with the rest of the evidence rather than afterwards,
// so the reference the caller keeps belongs to the same observation as the cwd
// and the argv it reports.
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
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return Identity{}, fmt.Errorf("read child stat: %w", err)
	}
	parent, startTicks, err := statFields(string(stat))
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		PID:        pid,
		Cwd:        cwd,
		Argv:       splitNULArgv(raw),
		StartTicks: startTicks,
		ParentPID:  parent,
	}, nil
}
