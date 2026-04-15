package middleware

import (
	"context"
	"io"
)

// CommandSpec represents a platform-agnostic command execution request
type CommandSpec struct {
	Runner       string   // The base executable (e.g. "npm", "pip")
	Args         []string // Arguments for the runner
	Env          []string // Additional environment variables in KEY=VALUE form
	EnvIsolation bool     // If true, the provider should apply environment isolation (like NVM)
	Dir          string   // Working directory for the command; empty means caller's CWD
}

// ExecResult holds the structured output of a command execution with separate streams.
type ExecResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

// ProcessHandle represents a running background process
type ProcessHandle interface {
	Wait() error
	Kill() error
	GetPID() int
}

// PipedProcess represents a process with captured stdout+stderr accessible via a reader
type PipedProcess interface {
	ProcessHandle
	Stdout() io.Reader
}

// Process defines the cross-platform process execution interface
type Process interface {
	// Exec runs a command and returns combined stdout+stderr output
	Exec(spec CommandSpec) ([]byte, error)
	// ExecSeparate runs a command with context and returns separate stdout/stderr.
	// The context controls cancellation/timeout.
	ExecSeparate(ctx context.Context, spec CommandSpec) (*ExecResult, error)
	// Start launches a command in the background and returns a handle for supervision
	Start(spec CommandSpec) (ProcessHandle, error)
	// StartPiped launches a command with stdout+stderr piped to the returned reader
	StartPiped(spec CommandSpec) (PipedProcess, error)
	// RunPrivileged runs a command with elevated OS privileges (e.g. sudo on unix, runas on windows)
	RunPrivileged(spec CommandSpec) ([]byte, error)
	// HasExecutable reports whether the named binary exists anywhere in the host $PATH
	HasExecutable(name string) bool
	// SpawnPTY allocates a pseudo-terminal and attaches a process to it
	SpawnPTY() error
}
