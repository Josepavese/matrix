package agentprobe

import (
	"context"
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// ACPInitialize verifies that a stdio provider completes initialization on the
// generation it is configured to negotiate. It is the supervisor-facing form;
// the doctor uses ACPInitializeVersion to report which generation answered.
func ACPInitialize(ctx context.Context, endpoint middleware.ProtocolEndpoint) error {
	_, err := ACPInitializeVersion(ctx, endpoint)
	return err
}

// ACPInitializeVersion verifies the ACP initialize handshake and returns the
// generation the peer agreed to. An empty configured version negotiates the
// highest generation Matrix supports; a configured pin restricts the request,
// so doctor and runtime probe the same path an operation would take.
func ACPInitializeVersion(ctx context.Context, endpoint middleware.ProtocolEndpoint) (int, error) {
	requested, err := zedacp.ConfiguredProtocolVersion(endpoint.ProtocolVersion)
	if err != nil {
		return 0, providerfailure.NewPreflight("", endpoint, "initialize", err)
	}
	resolved, err := agentlaunch.ResolveEndpoint("", endpoint)
	if err != nil {
		return 0, err
	}
	endpoint = resolved.Endpoint
	if endpoint.Sandbox != nil && endpoint.Sandbox.Container != nil {
		return 0, fmt.Errorf("container sandbox handshake requires an explicit workspace; use matrix sandbox doctor --workspace")
	}
	command, args := agentlaunch.PrepareStdio(endpoint.Command, endpoint.Args, endpoint.EnvIsolation)
	transport, err := zedacp.NewStdioTransport(ctx, command, endpoint.Env, args...)
	if err != nil {
		return 0, providerfailure.NewPreflight("", endpoint, "initialize", err)
	}
	client := zedacp.NewClient(ctx, transport)
	defer client.Close()
	response, err := client.Initialize(ctx, zedacp.InitializeRequest{
		ProtocolVersion: requested,
		ClientInfo:      map[string]interface{}{"name": "matrix", "version": "1.0"},
	})
	if err != nil {
		return 0, providerfailure.NewPreflight("", endpoint, "initialize", fmt.Errorf("ACP initialize failed: %w", err))
	}
	agreed := client.AuthenticatedProtocolVersion()
	if err := zedacp.ValidateInitializeResponse(response, agreed); err != nil {
		return 0, providerfailure.NewPreflight("", endpoint, "initialize", err)
	}
	return agreed, nil
}
