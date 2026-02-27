//go:build linux || darwin

package exec

import (
	"fmt"
	goexec "os/exec"

	"github.com/jose/matrix-v2/internal/middleware"
)

// Provider implements middleware.Process for Unix-like systems (Linux, macOS).
// Privilege elevation is achieved via `sudo`.
type Provider struct{}

// NewProvider creates a new Unix-compatible process execution provider
func NewProvider() *Provider {
	return &Provider{}
}

// Exec runs a command without elevated privileges
func (p *Provider) Exec(spec middleware.CommandSpec) ([]byte, error) {
	cmd := goexec.Command(spec.Runner, spec.Args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, &middleware.Error{
			Code:    "ERR_EXEC_FAILED",
			Message: fmt.Sprintf("Execution failed for: %s", spec.Runner),
			Op:      "exec_unixlike.Exec",
			Err:     err,
		}
	}
	return out, nil
}

// RunPrivileged runs a command with root elevation via `sudo`.
// On Linux and macOS, `sudo` is the standard mechanism for privilege elevation.
func (p *Provider) RunPrivileged(spec middleware.CommandSpec) ([]byte, error) {
	sudoArgs := append([]string{spec.Runner}, spec.Args...)
	cmd := goexec.Command("sudo", sudoArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, &middleware.Error{
			Code:    "ERR_PRIV_EXEC_FAILED",
			Message: fmt.Sprintf("Privileged execution failed (sudo) for: %s", spec.Runner),
			Op:      "exec_unixlike.RunPrivileged",
			Err:     err,
		}
	}
	return out, nil
}

// HasExecutable reports whether the named binary exists in the host $PATH
func (p *Provider) HasExecutable(name string) bool {
	_, err := goexec.LookPath(name)
	return err == nil
}

// SpawnPTY allocates a pseudo-terminal — to be implemented by PTY provider
func (p *Provider) SpawnPTY() error {
	return &middleware.Error{
		Code:    "ERR_NOT_IMPLEMENTED",
		Message: "SpawnPTY not implemented in exec provider; use pty provider",
		Op:      "exec_unixlike.SpawnPTY",
	}
}
