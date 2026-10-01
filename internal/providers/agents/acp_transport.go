package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

type transportSpec struct {
	Protocol     string
	Address      string
	Command      string
	Args         []string
	Env          []string
	EnvIsolation bool
	// Cwd is the working directory the child process is started in. It is the
	// run's workspace, so it is resolved per run and never pinned globally.
	Cwd string
}

func createTransport(ctx context.Context, spec transportSpec) (middleware.AgentTransport, error) {
	switch strings.ToLower(spec.Protocol) {
	case "ws":
		addr := spec.Address
		if !strings.HasPrefix(addr, "ws://") && !strings.HasPrefix(addr, "wss://") {
			addr = "ws://" + addr
		}
		return NewWSTransport(ctx, addr)
	case "stdio", "acp":
		if err := verifyChildWorkspace(spec); err != nil {
			return nil, err
		}
		command, args := agentlaunch.PrepareStdio(spec.Command, spec.Args, spec.EnvIsolation)
		return zedacp.NewStdioTransportWith(ctx, command, zedacp.StdioSpawnSpec{Dir: spec.Cwd, Env: spec.Env}, args...)
	case "unix":
		return NewUnixTransport(ctx, spec.Address)
	default:
		return nil, fmt.Errorf("unsupported ACP transport: %s", spec.Protocol)
	}
}

// verifyChildWorkspace gates the directory a child process is started in, and
// its agreement with the workspace the agent is told to use.
//
// The run workspace is checked before the fork, because exec reports a missing
// or unusable directory only after the process exists. Then the workspace
// directory the agent's own arguments name is compared with it: a child running
// in one workspace while the agent is told to use another is a provider that
// cannot answer for the workspace Matrix is about to speak for, and the refusal
// names all three paths (child directory, declared directory, launch command)
// instead of forking a process that will fail later without saying why.
//
// Nothing here is agent-specific: the arguments are read as data.
func verifyChildWorkspace(spec transportSpec) error {
	if strings.TrimSpace(spec.Cwd) == "" {
		return nil
	}
	info, err := os.Stat(spec.Cwd)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("the path is not a directory")
		}
		return fmt.Errorf("agent workspace %s cannot be used as the child working directory: %w (agent cwd parameter %v, launch command %s)", spec.Cwd, err, spec.Args, spec.Command)
	}
	return verifyWorkspaceAgreement(spec)
}

// verifyWorkspaceAgreement refuses a launch whose agent arguments name a
// different workspace than the run uses. A declared directory that does not
// exist is not a workspace claim and leaves the run workspace in charge: the
// stat error is the answer to "is there a directory here", not a failure.
func verifyWorkspaceAgreement(spec transportSpec) error {
	declared, ok := declaredWorkspaceDir(spec.Args)
	if !ok || sameDirectory(declared, spec.Cwd) {
		return nil
	}
	if _, err := os.Stat(declared); err != nil {
		return nil //nolint:nilerr // an absent declared directory is not a claim on the workspace
	}
	return fmt.Errorf("agent workspace %s cannot be used: the agent is started with the workspace arguments %v, which name %s, while this run's workspace is %s; a child cannot serve a workspace it was told to ignore (launch command %s)", spec.Cwd, spec.Args, declared, spec.Cwd, spec.Command)
}

// declaredWorkspaceDir returns the workspace directory an agent's launch
// arguments name, if any. It reads the argument forms a command line uses to
// point a program at a directory, so Matrix can tell whether the endpoint the
// operator registered still points at the workspace the run is about to use.
func declaredWorkspaceDir(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	// Skip the program itself, but never skip a first argument that is already a
	// flag: `env -C /dir program` and `-C /dir program` both name the directory.
	start := 1
	if strings.HasPrefix(args[0], "-") {
		start = 0
	}
	for i := start; i < len(args); i++ {
		arg, next := strings.TrimSpace(args[i]), ""
		if i+1 < len(args) {
			next = args[i+1]
		}
		for _, flag := range workspaceDirFlags {
			if arg == flag {
				return next, next != ""
			}
			if value := strings.TrimPrefix(arg, flag+"="); value != arg {
				return value, value != ""
			}
		}
	}
	return "", false
}

// workspaceDirFlags are the command-line flags that hand a program the directory
// it should work in. They are spellings, not agent names: any agent program
// started with one of them is declaring the same thing about itself.
var workspaceDirFlags = []string{"--cwd", "--chdir", "-C", "-w", "--working-directory"}

// sameDirectory compares two paths as directories: cleaned and resolved, so a
// trailing separator or a symlinked workspace is not mistaken for a different
// one.
func sameDirectory(first, second string) bool {
	resolve := func(value string) string {
		cleaned := filepath.Clean(value)
		if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
			return resolved
		}
		return cleaned
	}
	return resolve(first) == resolve(second)
}
