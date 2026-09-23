package agents

import (
	"context"

	"github.com/Josepavese/matrix/internal/middleware"
)

type ACPClient = acpClient

func NewACPClient(ctx context.Context, transport middleware.AgentTransport) ACPClient {
	return defaultACPSDK.NewClient(ctx, transport)
}
