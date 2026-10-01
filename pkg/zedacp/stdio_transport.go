package zedacp

import (
	"context"

	"github.com/Josepavese/matrix/pkg/zedacpstdio"
)

type StdioTransport = zedacpstdio.Transport

// StdioSpawnSpec is the launch half of a stdio agent connection: the working
// directory of the child process and the environment it inherits.
type StdioSpawnSpec = zedacpstdio.SpawnSpec

func NewStdioTransport(ctx context.Context, executable string, env []string, args ...string) (*StdioTransport, error) {
	return zedacpstdio.New(ctx, executable, StdioSpawnSpec{Env: env}, args...)
}

// NewStdioTransportWith starts the agent program with an explicit launch spec.
// Matrix passes the run's workspace as the working directory here, so the child,
// the protocol workspace and the agent's own cwd parameter can be made to agree,
// instead of pinning one directory for every run.
func NewStdioTransportWith(ctx context.Context, executable string, spec StdioSpawnSpec, args ...string) (*StdioTransport, error) {
	return zedacpstdio.New(ctx, executable, spec, args...)
}
