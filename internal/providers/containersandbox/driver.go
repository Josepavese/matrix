// Package containersandbox supplies the same explicit Docker CLI boundary on
// Linux, Windows and macOS. The engine must run local Linux containers.
package containersandbox

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/childenv"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

const GuestWorkspace = "/workspace"

type Launch struct {
	Policy    middleware.ContainerSandbox
	Workspace string
	Command   string
	Args      []string
	Env       []string
	Identity  string
}

type Transport struct {
	middleware.AgentTransport
	engine   string
	name     string
	once     sync.Once
	err      error
	done     chan struct{}
	evidence middleware.SandboxExecutionEvidence
}

// Start creates first, then attaches. Cleanup therefore owns an existing named
// container even if process startup or attachment fails, eliminating the race
// between killing a docker-run client and the daemon creating its container.
func Start(ctx context.Context, launch Launch) (*Transport, error) {
	owner, err := prepare(ctx, launch)
	if err != nil {
		return nil, err
	}
	stream, err := zedacp.NewStdioTransportWith(ctx, owner.engine, zedacp.StdioSpawnSpec{Env: childenv.Environment()}, "start", "--attach", "--interactive", owner.name)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("container sandbox attachment failed"), owner.Close())
	}
	owner.AgentTransport = stream
	go func() {
		select {
		case <-ctx.Done():
			_ = owner.Close()
		case <-owner.done:
		}
	}()
	return owner, nil
}

func prepare(ctx context.Context, launch Launch) (*Transport, error) {
	if _, err := agentlaunch.ReadSandbox(middleware.ProtocolEndpoint{Sandbox: &middleware.SandboxPolicy{Container: &launch.Policy}}); err != nil {
		return nil, err
	}
	engine, err := exec.LookPath(launch.Policy.Engine)
	if err != nil {
		return nil, fmt.Errorf("container sandbox engine unavailable")
	}
	image, err := inspectLocalImage(ctx, engine, launch.Policy.Image)
	if err != nil {
		return nil, err
	}
	workspace, state, err := mountPaths(launch.Workspace, launch.Policy.StateDir)
	if err != nil {
		return nil, err
	}
	state, err = prepareStateDirectory(state, workspace, launch.Identity, launch.Command)
	if err != nil {
		return nil, err
	}
	name := "matrix-sandbox-" + strings.ToLower(rand.Text())
	args, env, err := createArguments(launch, boundaryPaths{name, image, workspace, state})
	if err != nil {
		return nil, err
	}
	owner := &Transport{engine: engine, name: name, done: make(chan struct{})}
	if _, err := engineCommand(ctx, engine, env, args...); err != nil {
		return nil, errors.Join(fmt.Errorf("container sandbox creation failed: %w", err), owner.Close())
	}
	if err := verifyBoundary(ctx, engine, name, launch.Policy); err != nil {
		return nil, errors.Join(err, owner.Close())
	}
	owner.evidence = middleware.SandboxExecutionEvidence{Mechanism: "local-docker-linux-containers", Verification: "engine_configuration_verified", ImageID: image, WorkspaceAccess: launch.Policy.WorkspaceAccess, Network: launch.Policy.Network}
	return owner, nil
}

func (t *Transport) SandboxExecutionEvidence() middleware.SandboxExecutionEvidence { return t.evidence }

func (t *Transport) Close() error {
	t.once.Do(func() {
		defer close(t.done)
		// Remove through the daemon before terminating the attached client. Never
		// use a name from the request: only the random name created by this owner.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, t.err = engineCommand(ctx, t.engine, childenv.Environment(), "rm", "--force", t.name)
		if t.AgentTransport != nil {
			t.err = errors.Join(t.err, t.AgentTransport.Close())
		}
	})
	return t.err
}

func engineCommand(ctx context.Context, engine string, env []string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, engine, args...)
	cmd.Env = env
	cmd.WaitDelay = time.Second
	// No daemon error bodies, env values, image diagnostics or credentials are
	// returned to a channel. Machine-readable stdout is bounded before decoding.
	output := &boundedOutput{}
	cmd.Stdout = output
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("sandbox engine operation failed (%s)", args[0])
	}
	if output.overflow {
		return nil, fmt.Errorf("sandbox engine response exceeds limit")
	}
	return output.data, nil
}
