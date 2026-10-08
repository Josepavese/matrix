package middleware

// SandboxPolicy is an explicit execution contract, independent of provider ID.
// Native permissions configure tool decisions; Container supplies OS isolation.
type SandboxPolicy struct {
	NativeContract string            `json:"native_contract,omitempty"`
	NativeProfile  string            `json:"native_profile,omitempty"`
	Container      *ContainerSandbox `json:"container,omitempty"`
}

// ContainerSandbox requires an existing engine, cached Linux image, and explicit
// persistent state directory. Matrix never installs prerequisites or pulls images.
type ContainerSandbox struct {
	Engine          string `json:"engine"`
	Image           string `json:"image"`
	WorkspaceAccess string `json:"workspace_access"`
	Network         string `json:"network"`
	StateDir        string `json:"state_dir"`
	User            string `json:"user"`
	MemoryBytes     uint64 `json:"memory_bytes"`
	CPUs            uint32 `json:"cpus"`
	Pids            uint32 `json:"pids"`
}

// SandboxExecutionEvidence is produced after inspecting an owned container.
// It attests engine settings/capabilities, not provider tool-policy precedence.
type SandboxExecutionEvidence struct {
	Mechanism       string `json:"mechanism"`
	Verification    string `json:"verification"`
	ImageID         string `json:"image_id"`
	WorkspaceAccess string `json:"workspace_access"`
	Network         string `json:"network"`
}

type SandboxExecutionReporter interface {
	SandboxExecutionEvidence() SandboxExecutionEvidence
}
