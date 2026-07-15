package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

type transportSpec struct {
	Protocol     string
	Address      string
	Command      string
	Args         []string
	Env          []string
	EnvIsolation bool
}

func createTransport(ctx context.Context, spec transportSpec) (middleware.AgentTransport, error) {
	switch strings.ToLower(spec.Protocol) {
	case "ws":
		addr := spec.Address
		if !strings.HasPrefix(addr, "ws://") && !strings.HasPrefix(addr, "wss://") {
			addr = "ws://" + addr
		}
		return NewWSTransport(ctx, addr)
	case "stdio", "acp":
		command, args := agentlaunch.PrepareStdio(spec.Command, spec.Args, spec.EnvIsolation)
		return NewStdioTransport(ctx, command, spec.Env, args...)
	case "unix":
		return NewUnixTransport(ctx, spec.Address)
	default:
		return nil, fmt.Errorf("unsupported ACP transport: %s", spec.Protocol)
	}
}
