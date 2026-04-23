//go:build windows

package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	goexec "os/exec"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
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
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}
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

// ExecSeparate runs a command with context and returns separate stdout/stderr.
func (p *Provider) ExecSeparate(ctx context.Context, spec middleware.CommandSpec) (*middleware.ExecResult, error) {
	var cmd *goexec.Cmd
	if ctx != nil {
		cmd = goexec.CommandContext(ctx, spec.Runner, spec.Args...)
	} else {
		cmd = goexec.Command(spec.Runner, spec.Args...)
	}
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*goexec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, &middleware.Error{
				Code:    "ERR_EXEC_FAILED",
				Message: fmt.Sprintf("Execution failed for: %s", spec.Runner),
				Op:      "exec_windows.ExecSeparate",
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

// RunPrivileged runs a command with elevation via PowerShell's Start-Process -Verb RunAs.
// On Windows, this is the standard UAC-compatible elevation mechanism.
func (p *Provider) RunPrivileged(spec middleware.CommandSpec) ([]byte, error) {
	// Build a properly escaped argument list for PowerShell
	escapedArgs := escapePowerShellArgs(spec.Runner, spec.Args)
	psCmd := fmt.Sprintf(
		"Start-Process -Verb RunAs -Wait -FilePath '%s' -ArgumentList '%s'",
		escapePowerShellString(spec.Runner),
		escapedArgs,
	)
	cmd := goexec.Command("powershell", "-NoProfile", "-Command", psCmd)
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}
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

// escapePowerShellString escapes a string for safe embedding in a PowerShell single-quoted string.
func escapePowerShellString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// escapePowerShellArgs builds a properly escaped argument list for PowerShell's -ArgumentList.
func escapePowerShellArgs(runner string, args []string) string {
	allArgs := append([]string{runner}, args...)
	var escaped []string
	for _, a := range allArgs {
		escaped = append(escaped, escapePowerShellString(a))
	}
	return strings.Join(escaped, " ")
}

// HasExecutable reports whether the named binary exists in the host $PATH
func (p *Provider) HasExecutable(name string) bool {
	_, err := goexec.LookPath(name)
	return err == nil
}

// processHandle implements middleware.ProcessHandle
type processHandle struct {
	cmd     *goexec.Cmd
	stdinWr *os.File
}

func (p *processHandle) Wait() error {
	defer func() {
		if p.stdinWr != nil {
			p.stdinWr.Close()
		}
	}()
	return p.cmd.Wait()
}

func (p *processHandle) Kill() error {
	if p.stdinWr != nil {
		p.stdinWr.Close()
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
	cmd := goexec.Command(spec.Runner, spec.Args...)
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	// Create a pipe for Stdin so that the child process does not receive an immediate EOF
	pr, pw, err := os.Pipe()
	if err == nil {
		cmd.Stdin = pr
	}

	if err := cmd.Start(); err != nil {
		if pw != nil {
			pw.Close()
		}
		return nil, &middleware.Error{
			Code:    "ERR_EXEC_START_FAILED",
			Message: fmt.Sprintf("Failed to start process: %s", spec.Runner),
			Op:      "exec_windows.Start",
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
		Op:      "exec_windows.SpawnPTY",
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
	cmd := goexec.Command(spec.Runner, spec.Args...)
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, &middleware.Error{
			Code:    "ERR_EXEC_PIPE_FAILED",
			Message: fmt.Sprintf("Failed to create pipe for: %s", spec.Runner),
			Op:      "exec_windows.StartPiped",
			Err:     err,
		}
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, &middleware.Error{
			Code:    "ERR_EXEC_START_FAILED",
			Message: fmt.Sprintf("Failed to start piped process: %s", spec.Runner),
			Op:      "exec_windows.StartPiped",
			Err:     err,
		}
	}
	go func() {
		cmd.Wait()
		pw.Close()
	}()

	return &pipedProcess{
		cmd:    cmd,
		stdout: pr,
	}, nil
}
