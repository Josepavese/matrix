package childidentity

import (
	"fmt"
	"strings"
)

// Identity is what the kernel reports about a process Matrix started.
//
// It is read from /proc rather than reconstructed from the endpoint Matrix
// configured, because those two disagree exactly when it matters: a launcher
// that chdirs, or a program that rewrites its own command line, still has to
// tell the kernel the truth about where it runs and what it was handed.
type Identity struct {
	PID  int      `json:"pid"`
	Cwd  string   `json:"cwd"`
	Argv []string `json:"argv"`

	// StartTicks is the process start time the kernel reports (field 22 of
	// /proc/<pid>/stat, in clock ticks since boot). A pid is a number the kernel
	// recycles; the start time belongs to one process only, so it is what tells a
	// later observation of the same number apart from a different process that
	// inherited it.
	StartTicks uint64 `json:"start_ticks,omitempty"`
	// ParentPID is the parent the kernel reports (field 4). For a child Matrix
	// started it is Matrix itself, which is the second thing a recycled pid
	// cannot claim by accident.
	ParentPID int `json:"parent_pid,omitempty"`
}

// reuseGuard decides whether an observation still describes the process a probe
// started. The pid alone never answers that: it is a number the kernel reuses,
// and a diagnostic that reports the wrong process is worse than one that reports
// nothing.
//
// Two facts from the kernel answer it. The parent must be the process that
// started the child, which a recycled number cannot fake silently; and the start
// time must be the one observed when the child was first seen, which catches the
// pid being reused between two reads of the same probe. A platform that reports
// no start time keeps the parent check and no weaker claim.
type reuseGuard struct {
	parent  int
	started uint64
	armed   bool
}

// refusedIdentity marks a terminal refusal: the process observed is not the one
// the probe started, so waiting longer cannot make it appear.
type refusedIdentityError struct {
	message string
}

func (e *refusedIdentityError) Error() string { return e.message }

// refusedParent builds the terminal error for an observation that belongs to
// another process.
func refusedParent(pid, parent, want int) error {
	return &refusedIdentityError{message: fmt.Sprintf(
		"pid %d is not the child this probe started: its parent is %d, not %d", pid, parent, want)}
}

// refusedReuse builds the terminal error for a pid whose start time changed
// between two observations.
func refusedReuse(pid int, now, first uint64) error {
	return &refusedIdentityError{message: fmt.Sprintf(
		"pid %d was recycled: start time %d is not the one first observed (%d)", pid, now, first)}
}

func (g *reuseGuard) accept(identity Identity) error {
	if g.parent > 0 && identity.ParentPID > 0 && identity.ParentPID != g.parent {
		return refusedParent(identity.PID, identity.ParentPID, g.parent)
	}
	if !g.armed {
		g.started = identity.StartTicks
		g.armed = true
		return nil
	}
	if identity.StartTicks != g.started {
		return refusedReuse(identity.PID, identity.StartTicks, g.started)
	}
	return nil
}

// splitNULArgv splits a /proc/<pid>/cmdline payload. The kernel separates
// arguments with NUL bytes and terminates the list with one, so empty fields
// carry no argument and are dropped.
func splitNULArgv(raw []byte) []string {
	fields := strings.Split(string(raw), "\x00")
	argv := make([]string, 0, len(fields))
	for _, field := range fields {
		if field != "" {
			argv = append(argv, field)
		}
	}
	return argv
}

// statFields reads the parent and the start time out of a /proc/<pid>/stat
// payload. The second field is the executable name in parentheses and may itself
// contain spaces and parentheses, so the fields that carry a position are the
// ones after the LAST closing parenthesis: state is field 3, parent field 4 and
// start time field 22.
func statFields(raw string) (parent int, startTicks uint64, err error) {
	end := strings.LastIndex(raw, ")")
	if end < 0 || end+2 > len(raw) {
		return 0, 0, fmt.Errorf("malformed process stat: no command field")
	}
	fields := strings.Fields(raw[end+1:])
	if len(fields) < 20 {
		return 0, 0, fmt.Errorf("malformed process stat: %d fields after the command", len(fields))
	}
	if _, err := fmt.Sscanf(fields[1], "%d", &parent); err != nil {
		return 0, 0, fmt.Errorf("malformed process stat: parent %q: %w", fields[1], err)
	}
	if _, err := fmt.Sscanf(fields[19], "%d", &startTicks); err != nil {
		return 0, 0, fmt.Errorf("malformed process stat: start time %q: %w", fields[19], err)
	}
	return parent, startTicks, nil
}
