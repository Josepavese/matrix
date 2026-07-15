package zedacp

import (
	"context"

	"github.com/Josepavese/matrix/pkg/zedacpstdio"
)

type StdioTransport = zedacpstdio.Transport

func NewStdioTransport(ctx context.Context, executable string, env []string, args ...string) (*StdioTransport, error) {
	return zedacpstdio.New(ctx, executable, env, args...)
}
