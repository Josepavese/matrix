package agents

import (
	"context"

	"github.com/Josepavese/matrix/pkg/zedacp"
)

type WSTransport = zedacp.WSTransport

func NewWSTransport(ctx context.Context, url string) (*WSTransport, error) {
	return zedacp.NewWSTransport(ctx, url)
}
