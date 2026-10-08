package agentlaunch

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

const SandboxEnv = "MATRIX_SANDBOX"

func resolveSandbox(result Resolution) (Resolution, error) {
	policy, err := ReadSandbox(result.Endpoint)
	if err != nil || policy == nil {
		return result, err
	}
	if result.Endpoint.Kind != middleware.ProtocolKindACP || !sandboxStdio(result.Endpoint.Transport) {
		return result, fmt.Errorf("sandbox requires a local ACP stdio endpoint; remote isolation cannot be inferred")
	}
	if policy.NativeContract != "" {
		result.Endpoint, err = applyNativeSandbox(result.Endpoint, *policy)
		if err != nil {
			return result, err
		}
	}
	result.Endpoint.Sandbox = policy
	encoded, _ := json.Marshal(policy)
	result.Endpoint.Env = upsertEnv(result.Endpoint.Env, SandboxEnv, string(encoded))
	if result.Metadata == nil {
		result.Metadata = map[string]interface{}{}
	}
	result.Metadata["sandbox"] = map[string]interface{}{
		"native_contract": policy.NativeContract, "native_profile": policy.NativeProfile,
		"native_status": "configured_not_attested", "os_requested": policy.Container != nil,
		"host_acp_tools": "disabled", "automatic_permission_mode": "disabled",
	}
	return result, nil
}

// ReadSandbox also validates direct factory callers. Unset preserves existing
// provider behavior; malformed or unsupported requested policy never downgrades.
func ReadSandbox(endpoint middleware.ProtocolEndpoint) (*middleware.SandboxPolicy, error) {
	policy := cloneSandboxPolicy(endpoint.Sandbox)
	if raw := envValue(endpoint.Env, SandboxEnv); raw != "" {
		if len(raw) > 16384 {
			return nil, fmt.Errorf("sandbox declaration exceeds limit")
		}
		if err := uniqueSandboxObject([]byte(raw), 0); err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		policy = &middleware.SandboxPolicy{}
		if err := decoder.Decode(policy); err != nil {
			return nil, fmt.Errorf("invalid sandbox declaration")
		}
		if err := decoder.Decode(new(interface{})); err != io.EOF {
			return nil, fmt.Errorf("invalid sandbox trailing data")
		}
	}
	if policy == nil {
		return nil, nil
	}
	if err := validateSandbox(*policy); err != nil {
		return nil, err
	}
	return policy, nil
}

func cloneSandboxPolicy(policy *middleware.SandboxPolicy) *middleware.SandboxPolicy {
	if policy == nil {
		return nil
	}
	policyCopy := *policy
	if policy.Container != nil {
		container := *policy.Container
		policyCopy.Container = &container
	}
	return &policyCopy
}

func sandboxStdio(transport string) bool {
	return transport == "stdio" || transport == "acp"
}

func validateSandbox(policy middleware.SandboxPolicy) error {
	if policy.NativeContract == "" && policy.Container == nil {
		return fmt.Errorf("sandbox declaration must request a mechanism")
	}
	if policy.NativeContract == "" && policy.NativeProfile != "" {
		return fmt.Errorf("native profile requires its provider contract")
	}
	if policy.NativeContract != "" {
		if _, ok := nativePermissionEnvironment[policy.NativeContract]; !ok {
			return fmt.Errorf("unsupported native sandbox contract")
		}
		if policy.NativeProfile != "read-only" && policy.NativeProfile != "workspace" {
			return fmt.Errorf("unsupported native sandbox profile")
		}
	}
	if policy.Container != nil {
		return validateContainer(*policy.Container)
	}
	return nil
}

func validateContainer(policy middleware.ContainerSandbox) error {
	if missingContainerFields(policy) {
		return fmt.Errorf("container sandbox requires engine, image, state_dir and numeric user")
	}
	if strings.HasPrefix(policy.Image, "-") || strings.ContainsAny(policy.Image, "\x00\r\n") {
		return fmt.Errorf("invalid container image")
	}
	if policy.WorkspaceAccess != "read-only" && policy.WorkspaceAccess != "workspace-write" {
		return fmt.Errorf("invalid container workspace access")
	}
	if policy.Network != "none" && policy.Network != "bridge" {
		return fmt.Errorf("container network must be none or bridge")
	}
	if err := validateContainerResources(policy); err != nil {
		return err
	}
	return validateContainerUser(policy.User)
}

// ValidateContainerSandbox validates the shared agent/validator PAL contract.
func ValidateContainerSandbox(policy middleware.ContainerSandbox) error {
	return validateContainer(policy)
}

func validateContainerResources(policy middleware.ContainerSandbox) error {
	if policy.MemoryBytes < 16*1024*1024 || policy.MemoryBytes > 256*1024*1024*1024 {
		return fmt.Errorf("container memory limit must be 16 MiB to 256 GiB")
	}
	if policy.CPUs < 1 || policy.CPUs > 64 {
		return fmt.Errorf("container CPU limit must be 1 to 64")
	}
	if policy.Pids < 16 || policy.Pids > 4096 {
		return fmt.Errorf("container PID limit must be 16 to 4096")
	}
	return nil
}

func validateContainerUser(user string) error {
	parts := strings.Split(user, ":")
	if len(parts) != 2 {
		return fmt.Errorf("container user must be uid:gid")
	}
	for _, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" || strings.Trim(part, "0") == "" {
			return fmt.Errorf("container requires a non-root numeric user")
		}
	}
	return nil
}

func missingContainerFields(policy middleware.ContainerSandbox) bool {
	for _, value := range []string{policy.Engine, policy.Image, policy.StateDir, policy.User} {
		if value == "" {
			return true
		}
	}
	return false
}
