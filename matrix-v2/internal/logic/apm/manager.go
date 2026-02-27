package apm

import (
	"fmt"
	"log/slog"

	"github.com/jose/matrix-v2/internal/middleware"
)

// Package defines a mapped AI agent ecosystem package
type Package struct {
	Runner       string // e.g. "npm", "pip"
	InstallArgs  []string
	UpdateArgs   []string
	RemoveArgs   []string
	Binary       string   // The executable name we expect in $PATH
	Dependencies []string // e.g. []string{"npm", "node"}
	NeedsPriv    bool     // If true, calls proc.RunPrivileged instead of proc.Exec
}

// Registry maps logical agent names to their native ecosystem installers
var Registry = map[string]Package{
	"gemini": {
		Runner:       "npm",
		InstallArgs:  []string{"install", "-g", "@google/gemini-cli"},
		UpdateArgs:   []string{"update", "-g", "@google/gemini-cli"},
		RemoveArgs:   []string{"uninstall", "-g", "@google/gemini-cli"},
		Binary:       "gemini",
		Dependencies: []string{"npm", "node"},
		NeedsPriv:    true,
	},
	"codex": {
		Runner:       "npm",
		InstallArgs:  []string{"install", "-g", "@openai/codex"},
		UpdateArgs:   []string{"update", "-g", "@openai/codex"},
		RemoveArgs:   []string{"uninstall", "-g", "@openai/codex"},
		Binary:       "codex",
		Dependencies: []string{"npm", "node"},
		NeedsPriv:    true,
	},
	"opencode": {
		Runner:       "npm",
		InstallArgs:  []string{"install", "-g", "opencode-ai"},
		UpdateArgs:   []string{"update", "-g", "opencode-ai"},
		RemoveArgs:   []string{"uninstall", "-g", "opencode-ai"},
		Binary:       "opencode",
		Dependencies: []string{"npm", "node"},
		NeedsPriv:    true,
	},
	"claude": {
		Runner:       "npm",
		InstallArgs:  []string{"install", "-g", "@anthropic-ai/claude-cli"},
		UpdateArgs:   []string{"update", "-g", "@anthropic-ai/claude-cli"},
		RemoveArgs:   []string{"uninstall", "-g", "@anthropic-ai/claude-cli"},
		Binary:       "claude",
		Dependencies: []string{"npm", "node"},
		NeedsPriv:    true,
	},
	"kimi": {
		Runner:       "pip",
		InstallArgs:  []string{"install", "kimi-cli"},
		UpdateArgs:   []string{"install", "--upgrade", "kimi-cli"},
		RemoveArgs:   []string{"uninstall", "-y", "kimi-cli"},
		Binary:       "kimi",
		Dependencies: []string{"pip", "python3"},
		NeedsPriv:    true,
	},
}

// Manager handles AI Package Management (APM) operations.
// It is intentionally unaware of any OS-specific mechanisms.
// Platform concerns (sudo, runas) are delegated to the injected proc.
type Manager struct {
	proc middleware.Process
}

// NewManager creates a new APM logic manager with the given platform process runner
func NewManager(proc middleware.Process) *Manager {
	return &Manager{proc: proc}
}

// checkDependencies verifies that the required host executables are present
func (m *Manager) checkDependencies(pkgName string, deps []string) error {
	for _, dep := range deps {
		if !m.proc.HasExecutable(dep) {
			return &middleware.Error{
				Code:    "ERR_APM_MISSING_DEPS",
				Message: fmt.Sprintf("Cannot orchestrate '%s': missing required host dependency '%s'", pkgName, dep),
				Op:      "apm.checkDependencies",
			}
		}
	}
	return nil
}

// run dispatches a command to the correct execution path based on privilege requirements
func (m *Manager) run(pkg Package, args []string) error {
	spec := middleware.CommandSpec{Runner: pkg.Runner, Args: args}
	var (
		out []byte
		err error
	)
	if pkg.NeedsPriv {
		out, err = m.proc.RunPrivileged(spec)
	} else {
		out, err = m.proc.Exec(spec)
	}
	if err != nil {
		return &middleware.Error{
			Code:    "ERR_APM_EXEC",
			Message: string(out),
			Op:      "apm.run",
			Err:     err,
		}
	}
	return nil
}

// Install maps an agent name to an ecosystem installer and executes it globally
func (m *Manager) Install(pkgName string) error {
	pkg, exists := Registry[pkgName]
	if !exists {
		return &middleware.Error{
			Code:    "ERR_APM_UNKNOWN",
			Message: fmt.Sprintf("Unknown package: '%s'. Not found in APM registry.", pkgName),
			Op:      "apm.Install",
		}
	}
	if err := m.checkDependencies(pkgName, pkg.Dependencies); err != nil {
		return err
	}
	slog.Info("APM Install", "package", pkgName, "runner", pkg.Runner, "privileged", pkg.NeedsPriv)
	if err := m.run(pkg, pkg.InstallArgs); err != nil {
		return &middleware.Error{Code: "ERR_APM_INSTALL", Message: err.Error(), Op: "apm.Install"}
	}
	return nil
}

// Uninstall maps an agent name to an ecosystem uninstaller and executes it globally
func (m *Manager) Uninstall(pkgName string) error {
	pkg, exists := Registry[pkgName]
	if !exists {
		return &middleware.Error{
			Code:    "ERR_APM_UNKNOWN",
			Message: fmt.Sprintf("Unknown package: '%s'. Not found in APM registry.", pkgName),
			Op:      "apm.Uninstall",
		}
	}
	if err := m.checkDependencies(pkgName, pkg.Dependencies); err != nil {
		return err
	}
	slog.Info("APM Uninstall", "package", pkgName, "runner", pkg.Runner, "privileged", pkg.NeedsPriv)
	if err := m.run(pkg, pkg.RemoveArgs); err != nil {
		return &middleware.Error{Code: "ERR_APM_UNINSTALL", Message: err.Error(), Op: "apm.Uninstall"}
	}
	return nil
}

// Update maps an agent name to an ecosystem updater and executes it globally
func (m *Manager) Update(pkgName string) error {
	pkg, exists := Registry[pkgName]
	if !exists {
		return &middleware.Error{
			Code:    "ERR_APM_UNKNOWN",
			Message: fmt.Sprintf("Unknown package: '%s'. Not found in APM registry.", pkgName),
			Op:      "apm.Update",
		}
	}
	if err := m.checkDependencies(pkgName, pkg.Dependencies); err != nil {
		return err
	}
	slog.Info("APM Update", "package", pkgName, "runner", pkg.Runner, "privileged", pkg.NeedsPriv)
	if err := m.run(pkg, pkg.UpdateArgs); err != nil {
		return &middleware.Error{Code: "ERR_APM_UPDATE", Message: err.Error(), Op: "apm.Update"}
	}
	return nil
}

// List returns the names of all registry packages currently installed in the host $PATH
func (m *Manager) List() []string {
	var installed []string
	for name, pkg := range Registry {
		if m.proc.HasExecutable(pkg.Binary) {
			installed = append(installed, name)
		}
	}
	return installed
}
