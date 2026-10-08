package containersandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/childenv"
	"github.com/Josepavese/matrix/internal/middleware"
)

func inspectLocalImage(ctx context.Context, engine, image string) (string, error) {
	endpoint, err := engineCommand(ctx, engine, childenv.Environment(), "context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
	if err != nil {
		return "", err
	}
	address := strings.TrimSpace(string(endpoint))
	if !strings.HasPrefix(address, "unix://") && !strings.HasPrefix(address, "npipe://") {
		return "", fmt.Errorf("sandbox requires a local engine endpoint")
	}
	if err := verifyEngineLimits(ctx, engine); err != nil {
		return "", err
	}
	output, err := engineCommand(ctx, engine, childenv.Environment(), "image", "inspect", "--format", "{{json .}}", image)
	if err != nil {
		return "", fmt.Errorf("sandbox image unavailable locally; automatic pull is disabled")
	}
	var info struct {
		ID     string
		Os     string
		Config struct{ Volumes map[string]interface{} }
	}
	if json.Unmarshal(output, &info) != nil || info.Os != "linux" || !strings.HasPrefix(info.ID, "sha256:") {
		return "", fmt.Errorf("sandbox requires a cached Linux container image")
	}
	if len(info.Config.Volumes) != 0 {
		return "", fmt.Errorf("sandbox image declares uncontrolled persistent volumes")
	}
	return info.ID, nil
}

func verifyBoundary(ctx context.Context, engine, name string, policy middleware.ContainerSandbox) error {
	output, err := engineCommand(ctx, engine, childenv.Environment(), "inspect", "--format", "{{json .HostConfig}}", name)
	if err != nil {
		return err
	}
	var cfg struct {
		ReadonlyRootfs bool
		Privileged     bool
		NetworkMode    string
		Memory         uint64
		MemorySwap     uint64
		NanoCpus       uint64
		PidsLimit      uint32
		CapDrop        []string
		SecurityOpt    []string
	}
	if json.Unmarshal(output, &cfg) != nil {
		return fmt.Errorf("sandbox boundary cannot be verified")
	}
	if !cfg.ReadonlyRootfs || cfg.Privileged || cfg.NetworkMode != policy.Network {
		return fmt.Errorf("sandbox root/network boundary was not applied")
	}
	if !matchesResources(cfg.Memory, cfg.MemorySwap, cfg.NanoCpus, policy) || cfg.PidsLimit != policy.Pids {
		return fmt.Errorf("sandbox resource limits were not applied")
	}
	if !contains(cfg.CapDrop, "ALL") || !contains(cfg.SecurityOpt, "no-new-privileges:true") {
		return fmt.Errorf("sandbox privilege boundary was not applied")
	}
	return verifyMountBoundary(ctx, engine, name, policy)
}

func contains(values []string, desired string) bool {
	for _, value := range values {
		if value == desired {
			return true
		}
	}
	return false
}

func matchesResources(memory, swap, cpus uint64, policy middleware.ContainerSandbox) bool {
	return memory == policy.MemoryBytes && swap == policy.MemoryBytes && cpus == uint64(policy.CPUs)*1000000000
}
