package agents

import (
	"context"

	"github.com/jose/matrix-v2/pkg/zedacp"
)

type StdioTransport = zedacp.StdioTransport

func NewStdioTransport(ctx context.Context, executable string, env []string, args ...string) (*StdioTransport, error) {
	return zedacp.NewStdioTransport(ctx, executable, env, args...)
}
