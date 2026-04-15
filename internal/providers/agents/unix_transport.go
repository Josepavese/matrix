package agents

import (
	"context"

	"github.com/jose/matrix-v2/pkg/zedacp"
)

type UnixTransport = zedacp.UnixTransport

func NewUnixTransport(ctx context.Context, socketPath string) (*UnixTransport, error) {
	return zedacp.NewUnixTransport(ctx, socketPath)
}
