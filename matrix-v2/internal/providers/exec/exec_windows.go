//go:build windows

package exec

import (
	"fmt"
	goexec "os/exec"

	"github.com/jose/matrix-v2/internal/middleware"
)

// Provider implements middleware.Process for Windows.
// Privilege elevation uses `runas` via PowerShell's Start-Process with -Verb RunAs.
type Provider struct{}

// NewProvider creates a new Windows-compatible process execution provider
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
			Op:      "exec_windows.Exec",
			Err:     err,
		}
	}
	return out, nil
}

// RunPrivileged runs a command with elevation via PowerShell's Start-Process -Verb RunAs.
// On Windows, this is the standard UAC-compatible elevation mechanism.
func (p *Provider) RunPrivileged(spec middleware.CommandSpec) ([]byte, error) {
	// Compose the inner command as a string for PowerShell
	innerArgs := spec.Runner
	for _, arg := range spec.Args {
		innerArgs += " " + arg
	}
	psCmd := fmt.Sprintf("Start-Process -Verb RunAs -Wait -FilePath '%s' -ArgumentList '%s'", spec.Runner, innerArgs)
	cmd := goexec.Command("powershell", "-NoProfile", "-Command", psCmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, &middleware.Error{
			Code:    "ERR_PRIV_EXEC_FAILED",
			Message: fmt.Sprintf("Privileged execution failed (runas) for: %s", spec.Runner),
			Op:      "exec_windows.RunPrivileged",
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
		Op:      "exec_windows.SpawnPTY",
	}
}
