package agentprobe

import (
	"context"
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// ACPInitialize verifies that a stdio provider completes protocol v1 initialization.
func ACPInitialize(ctx context.Context, endpoint middleware.ProtocolEndpoint) error {
	command, args := agentlaunch.PrepareStdio(endpoint.Command, endpoint.Args, endpoint.EnvIsolation)
	transport, err := zedacp.NewStdioTransport(ctx, command, endpoint.Env, args...)
	if err != nil {
		return providerfailure.NewPreflight("", endpoint, "initialize", err)
	}
	client := zedacp.NewClient(ctx, transport)
	defer client.Close()
	response, err := client.Initialize(ctx, zedacp.InitializeRequest{
		ProtocolVersion: 1,
		ClientInfo:      map[string]interface{}{"name": "matrix", "version": "1.0"},
	})
	if err != nil {
		return providerfailure.NewPreflight("", endpoint, "initialize", fmt.Errorf("ACP initialize failed: %w", err))
	}
	if response.ProtocolVersion != 1 {
		err = fmt.Errorf("ACP protocol version %d is not supported (matrix supports 1)", response.ProtocolVersion)
		return providerfailure.NewPreflight("", endpoint, "initialize", err)
	}
	return nil
}
