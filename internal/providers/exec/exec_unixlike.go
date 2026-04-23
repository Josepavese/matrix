//go:build linux || darwin

package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	goexec "os/exec"

	"errors"

	"github.com/Josepavese/matrix/internal/middleware"
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
	cmd := p.prepareCmd(spec, false)
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

// ExecSeparate runs a command with context and returns separate stdout/stderr.
// Uses prepareCmd for proper NVM isolation and env handling, then applies context.
func (p *Provider) ExecSeparate(ctx context.Context, spec middleware.CommandSpec) (*middleware.ExecResult, error) {
	cmd := p.prepareCmd(spec, false)
	if ctx != nil {
		cmd = goexec.CommandContext(ctx, cmd.Args[0], cmd.Args[1:]...)
		if spec.EnvIsolation {
			// Re-apply NVM isolation via bash -c
			nvmInit := `export NVM_DIR="$HOME/.nvm"; if [ -s "$NVM_DIR/nvm.sh" ]; then \. "$NVM_DIR/nvm.sh"; fi; `
			fullCmd := nvmInit + fmt.Sprintf("%q", spec.Runner)
			for _, arg := range spec.Args {
				fullCmd += " " + fmt.Sprintf("%q", arg)
			}
			cmd = goexec.CommandContext(ctx, "bash", "-c", fullCmd)
		}
		if len(spec.Env) > 0 {
			cmd.Env = append(os.Environ(), spec.Env...)
		}
		if spec.Dir != "" {
			cmd.Dir = spec.Dir
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *goexec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, &middleware.Error{
				Code:    "ERR_EXEC_FAILED",
				Message: fmt.Sprintf("Execution failed for: %s", spec.Runner),
				Op:      "exec_unixlike.ExecSeparate",
				Err:     err,
			}
		}
	}

	return &middleware.ExecResult{
		ExitCode: exitCode,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
	}, nil
}

// RunPrivileged runs a command with root elevation via `sudo`.
func (p *Provider) RunPrivileged(spec middleware.CommandSpec) ([]byte, error) {
	cmd := p.prepareCmd(spec, true)
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
	if err == nil {
		return true
	}
	// Also check in NVM environment if name is node/npm or an agent
	if name == "node" || name == "npm" || name == "codex" || name == "gemini" || name == "claude" || name == "opencode" {
		spec := middleware.CommandSpec{Runner: "which", Args: []string{name}, EnvIsolation: true}
		if _, err := p.Exec(spec); err == nil {
			return true
		}
	}
	return false
}

func (p *Provider) prepareCmd(spec middleware.CommandSpec, privileged bool) *goexec.Cmd {
	var cmd *goexec.Cmd
	switch {
	case spec.EnvIsolation:
		// Wrap with NVM sourcing. This requires bash -c.
		nvmInit := `export NVM_DIR="$HOME/.nvm"; if [ -s "$NVM_DIR/nvm.sh" ]; then \. "$NVM_DIR/nvm.sh"; fi; `
		fullCmd := nvmInit + fmt.Sprintf("%q", spec.Runner)
		for _, arg := range spec.Args {
			fullCmd += " " + fmt.Sprintf("%q", arg)
		}
		if privileged {
			cmd = goexec.Command("sudo", "bash", "-c", fullCmd)
		} else {
			cmd = goexec.Command("bash", "-c", fullCmd)
		}
	case privileged:
		sudoArgs := append([]string{spec.Runner}, spec.Args...)
		cmd = goexec.Command("sudo", sudoArgs...)
	default:
		cmd = goexec.Command(spec.Runner, spec.Args...)
	}
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}
	return cmd
}

// processHandle implements middleware.ProcessHandle
type processHandle struct {
	cmd     *goexec.Cmd
	stdinWr *os.File
}

func (p *processHandle) Wait() error {
	defer func() {
		if p.stdinWr != nil {
			_ = p.stdinWr.Close()
		}
	}()
	return p.cmd.Wait()
}

func (p *processHandle) Kill() error {
	if p.stdinWr != nil {
		_ = p.stdinWr.Close()
	}
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

func (p *processHandle) GetPID() int {
	if p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return -1
}

// Start launches a command in the background
func (p *Provider) Start(spec middleware.CommandSpec) (middleware.ProcessHandle, error) {
	cmd := p.prepareCmd(spec, false)

	// Background runtime processes must not write directly to the parent stdio.
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	// Create a pipe for Stdin so that the child process does not receive an immediate EOF
	pr, pw, err := os.Pipe()
	if err == nil {
		cmd.Stdin = pr
	}

	if err := cmd.Start(); err != nil {
		if pw != nil {
			_ = pw.Close()
		}
		return nil, &middleware.Error{
			Code:    "ERR_EXEC_START_FAILED",
			Message: fmt.Sprintf("Failed to start process: %s", spec.Runner),
			Op:      "exec_unixlike.Start",
			Err:     err,
		}
	}

	return &processHandle{
		cmd:     cmd,
		stdinWr: pw,
	}, nil
}

// SpawnPTY allocates a pseudo-terminal — to be implemented by PTY provider
func (p *Provider) SpawnPTY() error {
	return &middleware.Error{
		Code:    "ERR_NOT_IMPLEMENTED",
		Message: "SpawnPTY not implemented in exec provider; use pty provider",
		Op:      "exec_unixlike.SpawnPTY",
	}
}

// pipedProcess implements middleware.PipedProcess
type pipedProcess struct {
	cmd    *goexec.Cmd
	stdout io.Reader
}

func (pp *pipedProcess) Wait() error {
	return pp.cmd.Wait()
}

func (pp *pipedProcess) Kill() error {
	if pp.cmd.Process != nil {
		return pp.cmd.Process.Kill()
	}
	return nil
}

func (pp *pipedProcess) GetPID() int {
	if pp.cmd.Process != nil {
		return pp.cmd.Process.Pid
	}
	return -1
}

func (pp *pipedProcess) Stdout() io.Reader {
	return pp.stdout
}

// StartPiped starts a command with stdout+stderr combined into a pipe reader.
func (p *Provider) StartPiped(spec middleware.CommandSpec) (middleware.PipedProcess, error) {
	cmd := p.prepareCmd(spec, false)

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, &middleware.Error{
			Code:    "ERR_EXEC_PIPE_FAILED",
			Message: fmt.Sprintf("Failed to create pipe for: %s", spec.Runner),
			Op:      "exec_unixlike.StartPiped",
			Err:     err,
		}
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return nil, &middleware.Error{
			Code:    "ERR_EXEC_START_FAILED",
			Message: fmt.Sprintf("Failed to start piped process: %s", spec.Runner),
			Op:      "exec_unixlike.StartPiped",
			Err:     err,
		}
	}
	// Close the write end in the parent after the child has inherited it.
	// The child's copy remains open until the process exits.
	go func() {
		_ = cmd.Wait()
		_ = pw.Close()
	}()

	return &pipedProcess{
		cmd:    cmd,
		stdout: pr,
	}, nil
}
