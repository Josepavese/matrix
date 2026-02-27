package middleware

// CommandSpec represents a platform-agnostic command execution request
type CommandSpec struct {
	Runner string   // The base executable (e.g. "npm", "pip")
	Args   []string // Arguments for the runner
}

// Process defines the cross-platform process execution interface
type Process interface {
	// Exec runs a command and returns combined stdout+stderr output
	Exec(spec CommandSpec) ([]byte, error)
	// RunPrivileged runs a command with elevated OS privileges (e.g. sudo on unix, runas on windows)
	RunPrivileged(spec CommandSpec) ([]byte, error)
	// HasExecutable reports whether the named binary exists anywhere in the host $PATH
	HasExecutable(name string) bool
	// SpawnPTY allocates a pseudo-terminal and attaches a process to it
	SpawnPTY() error
}
