// Package agentmgr handles agent installation, registration, and lifecycle management.
package agentmgr

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentidentity"
	"github.com/Josepavese/matrix/internal/logic/agentinstall"
	"github.com/Josepavese/matrix/internal/logic/matrixhome"
	"github.com/Josepavese/matrix/internal/middleware"
)

// Installer orchestrates the process of installing an agent from the ACP Registry.
type Installer struct {
	net      middleware.Network
	archive  middleware.Archive
	storage  middleware.Storage
	fs       middleware.FS
	proc     middleware.Process
	registry *RegistryClient
	baseDir  string

	// progress receives human-readable progress lines. It is a field rather than
	// direct writes to stdout so that a caller offering machine-readable output
	// (a future `matrix install --json`) can send progress elsewhere and keep
	// stdout parseable.
	progress io.Writer

	// allowUnverified is the operator's explicit acceptance of a binary
	// distribution whose registry entry publishes no sha256. It is off by
	// default: an install with nothing to verify is refused, and only
	// `matrix install --allow-unverified` turns it on, for that install. The
	// override is recorded in the install evidence and logged when it is used.
	allowUnverified bool
}

// SetAllowUnverified turns the explicit opt-in for digest-less binary
// distributions on or off. It exists for the operator who has checked that the
// index publishes no digest and accepts the artifact unverified; leaving it
// alone keeps the fail-closed default.
func (inst *Installer) SetAllowUnverified(allow bool) {
	inst.allowUnverified = allow
}

// SetProgressWriter redirects human-readable install progress. A nil writer
// discards it, which is what a machine-readable caller wants.
func (inst *Installer) SetProgressWriter(w io.Writer) {
	inst.progress = w
}

// progressf writes one progress line, tolerating an installer built without a
// writer (the zero value is a silent installer, never a panicking one).
func (inst *Installer) progressf(format string, args ...any) {
	if inst.progress == nil {
		return
	}
	fmt.Fprintf(inst.progress, format, args...)
}

// InstallerConfig represents the dependencies for an Installer.
type InstallerConfig struct {
	Net      middleware.Network
	Archive  middleware.Archive
	Storage  middleware.Storage
	FS       middleware.FS
	Process  middleware.Process
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
		progress: os.Stdout,
		net:      cfg.Net,
		archive:  cfg.Archive,
		storage:  cfg.Storage,
		fs:       cfg.FS,
		proc:     cfg.Process,
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
	existing, err := agentcfg.LoadEntry(inst.storage, agentID)
	if err != nil {
		return err
	}
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

	cfg, verification, err := inst.installResolved(ctx, agentID, manifest, resolved)
	if err != nil {
		return err
	}

	// 4. Register in Vault
	entry := agentcfg.Entry{Config: cfg, Override: existing.Override}
	if err := agentcfg.SaveEntry(inst.storage, agentID, entry); err != nil {
		return err
	}

	// 5. Save metadata
	meta := agentcfg.Meta{
		ID:                   manifest.ID,
		Name:                 manifest.Name,
		Version:              manifest.Version,
		Description:          manifest.Description,
		Repository:           manifest.Repository,
		Website:              manifest.Website,
		Authors:              manifest.Authors,
		License:              manifest.License,
		Icon:                 manifest.Icon,
		DistTypes:            manifest.DistTypes(),
		ArtifactVerification: verification,
	}
	return agentcfg.SaveMeta(inst.storage, agentID, meta)
}

func (inst *Installer) installResolved(ctx context.Context, agentID string, manifest *AgentManifest, resolved *ResolvedDist) (agentcfg.Config, *agentcfg.ArtifactVerification, error) {
	if resolved.Type == "binary" {
		binaryPath, verification, err := inst.installBinary(ctx, manifest)
		return agentcfg.Config{Command: binaryPath, Kind: "acp", Transport: "stdio"}, verification, err
	}
	if resolved.Type != "npx" && resolved.Type != "uvx" {
		return agentcfg.Config{}, nil, fmt.Errorf("unsupported distribution type: %s", resolved.Type)
	}
	if manifest.Distribution.Npx == nil || !agentidentity.IsCanonicalCodexPackage(manifest.Distribution.Npx.Package) {
		inst.progressf("Registering %s agent '%s' (v%s) via %s\n", resolved.Type, manifest.ID, manifest.Version, resolved.Command)
		return agentcfg.Config{
			Command: resolved.Command, Args: resolved.Args, Env: resolved.Env,
			Kind: "acp", Transport: "stdio",
		}, artifactVerificationNotApplicable(), nil
	}
	target, err := agentinstall.AgentDir(inst.baseDir, agentID)
	if err != nil {
		return agentcfg.Config{}, nil, err
	}
	cfg, err := agentinstall.InstallCanonicalCodex(ctx, agentinstall.Config{
		FS: inst.fs, Process: inst.proc, Target: target,
		Package: manifest.Distribution.Npx.Package, Env: agentinstall.EnvSlice(manifest.Distribution.Npx.Env),
	})
	return cfg, artifactVerificationNotApplicable(), err
}

// installBinary handles the binary distribution flow: validate the launcher the
// index names, download, verify, extract, resolve path.
func (inst *Installer) installBinary(ctx context.Context, manifest *AgentManifest) (string, *agentcfg.ArtifactVerification, error) {
	dist, err := inst.registry.ResolveDistribution(manifest)
	if err != nil {
		return "", nil, err
	}
	agentPath, err := agentinstall.AgentDir(inst.baseDir, manifest.ID)
	if err != nil {
		return "", nil, err
	}

	// The launcher is resolved before the download: a cmd that escapes the
	// agent directory costs no bytes and leaves nothing behind.
	binaryPath, err := agentinstall.ResolveLauncherPath(agentPath, dist.Cmd)
	if err != nil {
		return "", nil, fmt.Errorf("refusing %s: %w", manifest.ID, err)
	}

	verification, err := inst.fetchVerifiedArchive(ctx, manifest, agentPath, dist)
	if err != nil {
		return "", nil, err
	}
	if err := inst.requireExtractedLauncher(manifest.ID, dist.Cmd, binaryPath); err != nil {
		return "", nil, err
	}

	return binaryPath, verification, nil
}

// requireExtractedLauncher proves the launcher the index names really is inside
// the extracted agent directory. It is the last gate before registration: a cmd
// that resolves inside the directory but matches nothing in the archive would
// otherwise register an agent that cannot start.
func (inst *Installer) requireExtractedLauncher(agentID, cmd, resolved string) error {
	info, err := inst.fs.Stat(resolved)
	if err != nil {
		return fmt.Errorf("agent %q: the registry index publishes cmd %q, but %s does not exist after extraction; refusing to register the agent", agentID, cmd, resolved)
	}
	if info.IsDir() {
		return fmt.Errorf("agent %q: the registry index publishes cmd %q, which resolves to the directory %s; refusing to register the agent", agentID, cmd, resolved)
	}
	return nil
}

// fetchVerifiedArchive downloads the artifact the index publishes, refuses it
// unless its digest matches, and only then extracts it into agentPath. The
// temporary download is always removed, on every outcome.
func (inst *Installer) fetchVerifiedArchive(ctx context.Context, manifest *AgentManifest, agentPath string, dist *BinaryDist) (*agentcfg.ArtifactVerification, error) {
	tmpFile, err := agentinstall.TempArchive(inst.fs.TempDir(), manifest.ID, manifest.Version, dist.Archive)
	if err != nil {
		return nil, err
	}

	inst.progressf("Downloading %s %s from %s...\n", manifest.ID, manifest.Version, dist.Archive)
	if err := inst.net.Download(ctx, dist.Archive, tmpFile); err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = inst.fs.RemoveAll(tmpFile) }()

	// The digest gate runs before anything is written to the agent directory,
	// so a rejected artifact leaves no half installation behind.
	verification, err := inst.verifyArtifact(manifest.ID, tmpFile, inst.registry.PlatformKey(), dist)
	if err != nil {
		return nil, err
	}

	inst.progressf("Extracting to %s...\n", agentPath)
	if err := inst.fs.MkdirAll(agentPath, 0755); err != nil {
		return nil, err
	}
	if err := inst.archive.Extract(tmpFile, agentPath); err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	return verification, nil
}

// Uninstall removes the agent's files and its registration from the Vault.
func (inst *Installer) Uninstall(_ context.Context, agentID string) error {
	// 1. Remove files
	agentPath, err := agentinstall.AgentDir(inst.baseDir, agentID)
	if err != nil {
		return err
	}
	if _, err := inst.fs.Stat(agentPath); err == nil {
		inst.progressf("Removing agent directory %s...\n", agentPath)
		if err := inst.fs.RemoveAll(agentPath); err != nil {
			return fmt.Errorf("failed to remove agent directory: %w", err)
		}
	}

	// 2. Remove config + metadata from Vault
	inst.progressf("Removing agent %s from Vault...\n", agentID)
	if err := agentcfg.DeleteEntry(inst.storage, agentID); err != nil {
		return fmt.Errorf("failed to remove agent config: %w", err)
	}
	if err := agentcfg.DeleteMeta(inst.storage, agentID); err != nil {
		slog.Warn("failed to delete agent metadata", "agent", agentID, "error", err)
	}
	return nil
}
