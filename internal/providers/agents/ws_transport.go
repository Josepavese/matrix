package agents

import (
	"context"

	"github.com/jose/matrix-v2/pkg/zedacp"
)

type WSTransport = zedacp.WSTransport

func NewWSTransport(ctx context.Context, url string) (*WSTransport, error) {
	return zedacp.NewWSTransport(ctx, url)
}
