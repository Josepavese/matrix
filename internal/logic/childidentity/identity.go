package childidentity

import "strings"

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
