package containersandbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/childenv"
	"github.com/Josepavese/matrix/internal/middleware"
)

func verifyMountBoundary(ctx context.Context, engine, name string, policy middleware.ContainerSandbox) error {
	output, err := engineCommand(ctx, engine, childenv.Environment(), "inspect", "--format", "{{json .}}", name)
	if err != nil {
		return err
	}
	var container struct {
		Config struct{ User string }
		Mounts []struct {
			Type, Destination string
			RW                bool
		}
		HostConfig struct{ LogConfig struct{ Type string } }
	}
	if json.Unmarshal(output, &container) != nil {
		return fmt.Errorf("sandbox mounts cannot be verified")
	}
	if container.Config.User != policy.User || container.HostConfig.LogConfig.Type != "none" {
		return fmt.Errorf("sandbox user/log retention was not applied")
	}
	if len(container.Mounts) != 2 {
		return fmt.Errorf("sandbox contains undeclared persistent mounts")
	}
	seen := map[string]bool{}
	for _, mount := range container.Mounts {
		if err := verifyMountDestination(mount.Type, mount.Destination, mount.RW, policy); err != nil {
			return err
		}
		seen[mount.Destination] = true
	}
	if len(seen) != 2 {
		return fmt.Errorf("sandbox mount destinations overlap")
	}
	return nil
}

func verifyMountDestination(kind, destination string, writable bool, policy middleware.ContainerSandbox) error {
	if kind != "bind" {
		return fmt.Errorf("sandbox persistent mount is not a declared bind")
	}
	switch destination {
	case GuestWorkspace:
		if writable != (policy.WorkspaceAccess == "workspace-write") {
			return fmt.Errorf("sandbox workspace access was not applied")
		}
	case "/home/matrix":
		if !writable {
			return fmt.Errorf("sandbox session state is not writable")
		}
	default:
		return fmt.Errorf("sandbox exposes an undeclared destination")
	}
	return nil
}
