package agentmgr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentinstall"
	"github.com/Josepavese/matrix/internal/logic/agentstore"
)

// RegistryManifest fetches the governed registry entry for one agent. It is the
// read path the store commands use for drift checks; the installer itself keeps
// using the registry client directly.
func (inst *Installer) RegistryManifest(ctx context.Context, agentID string) (*AgentManifest, error) {
	return inst.registry.FetchManifest(ctx, agentID)
}

// PlatformKey is the registry platform key the current build resolves
// distributions for (for example linux-x86_64).
func (inst *Installer) PlatformKey() string {
	return inst.registry.PlatformKey()
}

// PublishToStore downloads the binary artifact the governed registry publishes
// for agentID, verifies its digest against the index, and publishes it as an
// immutable, digest-addressed generation of an artifact store shared by many
// PAL homes. Publishing is the only operation that writes the store: homes then
// only read it, which is what keeps a consumer backup from carrying the same
// software once per worker.
//
// The store only carries verified bytes: an index entry with no published
// sha256 is refused rather than published with a weaker record, and a
// distribution type that downloads no artifact (npx/uvx) is not publishable.
func (inst *Installer) PublishToStore(ctx context.Context, agentID, storeRoot string) (agentstore.Generation, error) {
	manifest, err := inst.registry.FetchManifest(ctx, agentID)
	if err != nil {
		return agentstore.Generation{}, err
	}
	dist, published, args, err := inst.publishableArtifact(manifest)
	if err != nil {
		return agentstore.Generation{}, err
	}
	tmpFile, err := agentinstall.TempArchive(inst.fs.TempDir(), manifest.ID, manifest.Version, dist.Archive)
	if err != nil {
		return agentstore.Generation{}, err
	}
	defer func() { _ = inst.fs.RemoveAll(tmpFile) }()
	inst.progressf("Downloading %s %s from %s...\n", manifest.ID, manifest.Version, dist.Archive)
	if err := inst.net.Download(ctx, dist.Archive, tmpFile); err != nil {
		return agentstore.Generation{}, fmt.Errorf("download failed: %w", err)
	}
	if _, err := inst.verifyArtifact(manifest.ID, tmpFile, inst.registry.PlatformKey(), dist); err != nil {
		return agentstore.Generation{}, err
	}
	store, err := agentstore.OpenPublisher(storeRoot, inst.home)
	if err != nil {
		return agentstore.Generation{}, err
	}
	gen, err := store.PublishArchive(inst.archive, tmpFile, published, agentstore.Generation{
		AgentID:    manifest.ID,
		Version:    manifest.Version,
		Platform:   inst.registry.PlatformKey(),
		Artifact:   dist.Archive,
		Launcher:   dist.Cmd,
		Args:       args,
		License:    manifest.License,
		Repository: manifest.Repository,
	})
	if err != nil {
		return agentstore.Generation{}, err
	}
	inst.progressf("Published %s %s generation %s for %s\n", gen.AgentID, gen.Version, gen.Digest, gen.Platform)
	return gen, nil
}

// publishableArtifact is the gate that runs before any byte is downloaded: the
// distribution must be a binary archive and the index must publish a valid
// sha256 for it. The store only carries verified bytes, so an entry that would
// only be publishable unverified is refused here, not stored with a weaker
// record.
func (inst *Installer) publishableArtifact(manifest *AgentManifest) (*BinaryDist, string, []string, error) {
	dist, err := inst.registry.ResolveDistribution(manifest)
	if err != nil {
		return nil, "", nil, fmt.Errorf("agent %q has no binary distribution to publish into the store (npx/uvx agents are installed by their package manager, not from an archive): %w", manifest.ID, err)
	}
	published := strings.ToLower(strings.TrimSpace(dist.SHA256))
	if published == "" || !isHexSHA256(published) {
		return nil, "", nil, fmt.Errorf("refusing to publish %q: the registry index publishes no valid sha256 for %s (%s); the store only carries verified bytes", manifest.ID, dist.Archive, inst.registry.PlatformKey())
	}
	if err := inst.registry.validateArtifactURL(dist.Archive); err != nil {
		return nil, "", nil, err
	}
	resolved, err := inst.registry.ResolveAnyDistribution(manifest)
	if err != nil {
		return nil, "", nil, err
	}
	return dist, published, append([]string{}, resolved.Args...), nil
}

// ResolveStoreGeneration selects the generation an install may use when the
// caller named no digest: exactly one generation must match the agent, the
// current platform and the version the governed registry publishes today. No
// match is a named error with the command that publishes one, never a silent
// fallback to a different version.
func (inst *Installer) ResolveStoreGeneration(ctx context.Context, agentID, storeRoot string) (agentstore.Generation, error) {
	manifest, err := inst.registry.FetchManifest(ctx, agentID)
	if err != nil {
		return agentstore.Generation{}, err
	}
	store, err := agentstore.OpenConsumer(storeRoot, inst.home)
	if err != nil {
		return agentstore.Generation{}, err
	}
	generations, err := store.List()
	if err != nil {
		return agentstore.Generation{}, err
	}
	platform := inst.registry.PlatformKey()
	matches := matchStoreGenerations(generations, agentID, manifest.Version, platform)
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return agentstore.Generation{}, fmt.Errorf("the store %s has no verified generation for %s %s (%s); publish it first with `matrix agent store publish %s --store %s`", store.Root(), agentID, manifest.Version, platform, agentID, store.Root())
	default:
		return agentstore.Generation{}, fmt.Errorf("the store %s has %d generations for %s %s (%s); pass --digest with one of: %s", store.Root(), len(matches), agentID, manifest.Version, platform, storeDigestList(matches))
	}
}

func matchStoreGenerations(generations []agentstore.Generation, agentID, version, platform string) []agentstore.Generation {
	matches := make([]agentstore.Generation, 0, 1)
	for _, gen := range generations {
		if gen.AgentID == agentID && gen.Version == version && gen.Platform == platform {
			matches = append(matches, gen)
		}
	}
	return matches
}

func storeDigestList(generations []agentstore.Generation) string {
	digests := make([]string, 0, len(generations))
	for _, gen := range generations {
		digests = append(digests, gen.Digest)
	}
	return strings.Join(digests, ", ")
}

// InstallFromStore registers an agent whose software is a verified generation of
// the shared immutable store instead of a copy under this PAL home. It never
// touches the network: the generation is verified against the digest-addressed
// store, and the per-home reference is the only state written beside the Vault
// record.
func (inst *Installer) InstallFromStore(ctx context.Context, agentID, storeRoot, digest string) error {
	_ = ctx
	if strings.TrimSpace(inst.home) == "" {
		return fmt.Errorf("cannot record a store reference for %q: the installer has no PAL home", agentID)
	}
	store, err := agentstore.OpenConsumer(storeRoot, inst.home)
	if err != nil {
		return err
	}
	gen, err := store.Get(digest)
	if err != nil {
		return err
	}
	if gen.AgentID != agentID {
		return fmt.Errorf("store generation %s belongs to agent %q, not %q", gen.Digest, gen.AgentID, agentID)
	}
	platform := inst.registry.PlatformKey()
	if gen.Platform != platform {
		return fmt.Errorf("store generation %s was published for %s, but this host resolves %s; publish a generation for this platform", gen.Digest, gen.Platform, platform)
	}
	if err := store.Verify(digest); err != nil {
		return fmt.Errorf("store generation %s failed verification; refusing to install %q: %w", digest, agentID, err)
	}
	blobDir, err := store.BlobDir(digest)
	if err != nil {
		return err
	}
	launcher, err := storedLauncherPath(blobDir, gen.Launcher)
	if err != nil {
		return err
	}
	ref := agentstore.Reference{
		Store: store.Root(), Digest: gen.Digest, TreeManifestSHA256: gen.TreeManifestSHA256,
		Launcher: gen.Launcher, Version: gen.Version, Platform: gen.Platform, InstalledAt: time.Now().UTC(),
	}
	if err := agentstore.WriteReference(inst.home, agentID, ref); err != nil {
		return err
	}
	if err := inst.saveStoreRecord(agentID, gen, launcher, ref); err != nil {
		_ = agentstore.RemoveReference(inst.home, agentID)
		return err
	}
	inst.progressf("Installed %s %s from store generation %s (launcher %s)\n", gen.AgentID, gen.Version, gen.Digest, launcher)
	return nil
}

// storedLauncherPath resolves the generation launcher inside its published
// tree, refusing absolute or escaping values, and proves it is a regular file.
func storedLauncherPath(blobDir, launcher string) (string, error) {
	launcher = strings.TrimSpace(launcher)
	if launcher == "" || filepath.IsAbs(launcher) {
		return "", fmt.Errorf("store generation publishes launcher %q, which is empty or absolute; refusing to install", launcher)
	}
	clean := filepath.Clean(filepath.FromSlash(launcher))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("store generation launcher %q escapes the published tree; refusing to install", launcher)
	}
	path := filepath.Join(blobDir, clean)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("store generation launcher %s is missing: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("store generation launcher %s is a directory, not a program", path)
	}
	return path, nil
}

// saveStoreRecord writes the Vault config and metadata that point the agent at
// the store generation, keeping the operator override and the display metadata
// the previous install recorded. The reference file is written by the caller
// first: a stale reference only keeps generations alive, while a record without
// its reference could be reclaimed by the store's garbage collector.
func (inst *Installer) saveStoreRecord(agentID string, gen agentstore.Generation, launcher string, ref agentstore.Reference) error {
	existing, err := agentcfg.LoadEntry(inst.storage, agentID)
	if err != nil {
		return err
	}
	meta, err := agentcfg.LoadMeta(inst.storage, agentID)
	if err != nil {
		return err
	}
	cfg := agentcfg.Config{Command: launcher, Args: append([]string{}, gen.Args...), Kind: "acp", Transport: "stdio"}
	entry := agentcfg.Entry{Config: cfg, Override: existing.Override}
	if err := agentcfg.SaveEntry(inst.storage, agentID, entry); err != nil {
		return fmt.Errorf("failed to register the store-backed agent record: %w", err)
	}
	name := meta.Name
	if name == "" {
		name = agentID
	}
	reference := agentcfg.ArtifactReference{
		Store: ref.Store, Digest: ref.Digest, TreeManifestSHA256: ref.TreeManifestSHA256,
		Launcher: ref.Launcher, Version: ref.Version, Platform: ref.Platform, InstalledAt: ref.InstalledAt,
	}
	created := agentcfg.Meta{
		ID: agentID, Name: name, Version: gen.Version,
		Description: meta.Description, Repository: gen.Repository, Website: meta.Website,
		Authors: meta.Authors, License: gen.License, Icon: meta.Icon, DistTypes: meta.DistTypes,
		ArtifactVerification: &agentcfg.ArtifactVerification{
			Verified: true, Status: agentcfg.ArtifactVerified, Platform: gen.Platform,
			Artifact: gen.Artifact, Expected: gen.Digest, Actual: gen.Digest, VerifiedAt: gen.PublishedAt,
		},
		Reference: &reference,
	}
	return agentcfg.SaveMeta(inst.storage, agentID, created)
}

// removeStoreReference drops the per-home store reference of an agent that is
// being uninstalled. A home without a store reference is not an error: the agent
// may have been installed as a per-home copy, whose files are gone with the
// agent directory.
func (inst *Installer) removeStoreReference(agentID string) error {
	if strings.TrimSpace(inst.home) == "" {
		return nil
	}
	return agentstore.RemoveReference(inst.home, agentID)
}
