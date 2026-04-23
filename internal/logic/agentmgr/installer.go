// Package agentmgr handles agent installation, registration, and lifecycle management.
package agentmgr

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/matrixhome"
	"github.com/Josepavese/matrix/internal/middleware"
)

// Installer orchestrates the process of installing an agent from the ACP Registry.
type Installer struct {
	net      middleware.Network
	archive  middleware.Archive
	storage  middleware.Storage
	fs       middleware.FS
	registry *RegistryClient
	baseDir  string
}

// InstallerConfig represents the dependencies for an Installer.
type InstallerConfig struct {
	Net      middleware.Network
	Archive  middleware.Archive
	Storage  middleware.Storage
	FS       middleware.FS
	Registry *RegistryClient
	BaseDir  string
}

// NewInstaller creates a new agent Installer from the given config.
func NewInstaller(cfg InstallerConfig) (*Installer, error) {
	if cfg.BaseDir == "" {
		home, err := matrixhome.Resolve()
		if err != nil {
			return nil, fmt.Errorf("cannot determine matrix home for agent install path: %w", err)
		}
		cfg.BaseDir = matrixhome.AgentsDir(home)
	}
	return &Installer{
		net:      cfg.Net,
		archive:  cfg.Archive,
		storage:  cfg.Storage,
		fs:       cfg.FS,
		registry: cfg.Registry,
		baseDir:  cfg.BaseDir,
	}, nil
}

// RegistryClient returns the ACP registry client used by this installer.
func (inst *Installer) RegistryClient() *RegistryClient {
	return inst.registry
}

// Install fetches, downloads, extracts and registers an agent.
// Supports binary, npx, and uvx distribution types.
func (inst *Installer) Install(ctx context.Context, agentID string) error {
	// 1. Fetch Manifest
	manifest, err := inst.registry.FetchManifest(ctx, agentID)
	if err != nil {
		return err
	}

	// 2. Resolve best distribution
	resolved, err := inst.registry.ResolveAnyDistribution(manifest)
	if err != nil {
		return err
	}

	// 3. Install based on distribution type
	var cfg agentcfg.Config

	switch resolved.Type {
	case "binary":
		binaryPath, err := inst.installBinary(ctx, manifest)
		if err != nil {
			return err
		}
		cfg = agentcfg.Config{
			Command:   binaryPath,
			Kind:      "acp",
			Transport: "stdio",
		}
	case "npx", "uvx":
		fmt.Printf("Registering %s agent '%s' (v%s) via %s\n", resolved.Type, manifest.ID, manifest.Version, resolved.Command)
		cfg = agentcfg.Config{
			Command:   resolved.Command,
			Args:      resolved.Args,
			Env:       resolved.Env,
			Kind:      "acp",
			Transport: "stdio",
		}
	default:
		return fmt.Errorf("unsupported distribution type: %s", resolved.Type)
	}

	// 4. Register in Vault
	entry := agentcfg.Entry{Config: cfg}
	if err := agentcfg.SaveEntry(inst.storage, agentID, entry); err != nil {
		return err
	}

	// 5. Save metadata
	meta := agentcfg.Meta{
		ID:          manifest.ID,
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Repository:  manifest.Repository,
		Website:     manifest.Website,
		Authors:     manifest.Authors,
		License:     manifest.License,
		Icon:        manifest.Icon,
		DistTypes:   manifest.DistTypes(),
	}
	return agentcfg.SaveMeta(inst.storage, agentID, meta)
}

// installBinary handles the binary distribution flow: download, extract, resolve path.
func (inst *Installer) installBinary(ctx context.Context, manifest *AgentManifest) (string, error) {
	dist, err := inst.registry.ResolveDistribution(manifest)
	if err != nil {
		return "", err
	}

	agentDir := filepath.Join(inst.baseDir, manifest.ID)
	tmpFile := filepath.Join(inst.fs.TempDir(), fmt.Sprintf("matrix-agent-%s-%s", manifest.ID, manifest.Version))

	tmpFile += archiveExt(dist.Archive)

	fmt.Printf("Downloading %s %s from %s...\n", manifest.ID, manifest.Version, dist.Archive)
	if err := inst.net.Download(ctx, dist.Archive, tmpFile); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = inst.fs.RemoveAll(tmpFile) }()

	fmt.Printf("Extracting to %s...\n", agentDir)
	if err := inst.fs.MkdirAll(agentDir, 0755); err != nil {
		return "", err
	}
	if err := inst.archive.Extract(tmpFile, agentDir); err != nil {
		return "", fmt.Errorf("extraction failed: %w", err)
	}

	binaryPath := dist.Cmd
	if filepath.IsLocal(binaryPath) || (len(binaryPath) > 2 && binaryPath[:2] == "./") {
		binaryPath = filepath.Join(agentDir, binaryPath)
	}

	return binaryPath, nil
}

// Uninstall removes the agent's files and its registration from the Vault.
func (inst *Installer) Uninstall(_ context.Context, agentID string) error {
	// 1. Remove files
	agentDir := filepath.Join(inst.baseDir, agentID)
	if _, err := inst.fs.Stat(agentDir); err == nil {
		fmt.Printf("Removing agent directory %s...\n", agentDir)
		if err := inst.fs.RemoveAll(agentDir); err != nil {
			return fmt.Errorf("failed to remove agent directory: %w", err)
		}
	}

	// 2. Remove config + metadata from Vault
	fmt.Printf("Removing agent %s from Vault...\n", agentID)
	if err := agentcfg.DeleteEntry(inst.storage, agentID); err != nil {
		return fmt.Errorf("failed to remove agent config: %w", err)
	}
	if err := agentcfg.DeleteMeta(inst.storage, agentID); err != nil {
		slog.Warn("failed to delete agent metadata", "agent", agentID, "error", err)
	}
	return nil
}

// archiveExt derives the archive extension from a URL.
// Handles compound extensions like .tar.gz, .tar.bz2, .tar.xz.
func archiveExt(url string) string {
	lower := strings.ToLower(url)
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tgz", ".tbz2", ".txz", ".zip"} {
		if strings.HasSuffix(lower, ext) {
			return ext
		}
	}
	return filepath.Ext(url)
}
