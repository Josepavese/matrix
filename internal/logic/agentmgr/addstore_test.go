package agentmgr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentstore"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/middleware"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
	networkprovider "github.com/Josepavese/matrix/internal/providers/network"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// newStoreInstaller wires the production providers against an isolated vault, a
// PAL home with no software yet, and a separate temporary artifact store. The
// store lives outside the home on purpose: a store inside a home would defeat
// the backup contract it exists for.
func newStoreInstaller(t *testing.T, registryURL string) (*Installer, middleware.Storage, string, string) {
	t.Helper()

	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)

	store := memstore.New()
	home := t.TempDir()
	baseDir := filepath.Join(home, "agents")
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		t.Fatalf("cannot create the test agents dir: %v", err)
	}
	registry := NewRegistryClient(networkprovider.NewProvider(), registryURL)
	registry.goos = "linux"
	registry.arch = "amd64"
	installer, err := NewInstaller(InstallerConfig{
		Net:      networkprovider.NewProvider(),
		Archive:  osfs.NewArchiveProvider(),
		Storage:  store,
		FS:       osfs.NewFSProvider(),
		Process:  execprovider.NewProvider(),
		Registry: registry,
		BaseDir:  baseDir,
		Home:     home,
	})
	if err != nil {
		t.Fatalf("NewInstaller failed: %v", err)
	}
	return installer, store, home, tempDir
}

// sealStore turns a published store into the read-only shape a consumer sees:
// no directory or file in it is writable by the runtime user. Sealing removes
// write bits only, exactly like `chmod -R a-w`: the launcher stays executable,
// because a store that cannot run what it stores would only move the problem.
// The cleanup restores the write bits t.TempDir needs to remove the tree.
func sealStore(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		return os.Chmod(path, info.Mode().Perm()&^0o222)
	})
	if err != nil {
		t.Fatalf("cannot seal the test store: %v", err)
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil //nolint:nilerr // best-effort cleanup: an unreadable entry is not a test failure.
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				return nil //nolint:nilerr // best-effort cleanup: an unreadable entry is not a test failure.
			}
			perm := info.Mode().Perm()
			if entry.IsDir() {
				perm |= 0o700
			} else {
				perm |= 0o200
			}
			return os.Chmod(path, perm)
		})
	})
}

func TestPublishToStorePublishesVerifiedGeneration(t *testing.T) {
	registry := startFakeRegistry(t, publishRealDigest)
	installer, _, home, tempDir := newStoreInstaller(t, registry.registryURL())
	storeRoot := t.TempDir()

	gen, err := installer.PublishToStore(context.Background(), "opencode", storeRoot)
	if err != nil {
		t.Fatalf("publishing a verified artifact must succeed: %v", err)
	}
	if gen.Digest != registry.digest {
		t.Fatalf("generation digest = %q, want the published %q", gen.Digest, registry.digest)
	}
	if gen.AgentID != "opencode" || gen.Version != "9.9.9" || gen.Platform != testPlatform {
		t.Fatalf("generation identity = %+v", gen)
	}
	if gen.Launcher != "./opencode" {
		t.Fatalf("generation launcher = %q, want the index cmd", gen.Launcher)
	}
	if len(gen.Args) != 1 || gen.Args[0] != "acp" {
		t.Fatalf("generation args = %v, want the index args", gen.Args)
	}
	if gen.TreeManifestSHA256 == "" {
		t.Fatal("a generation must carry the digest of its tree manifest")
	}
	sealStore(t, storeRoot)
	consumer, err := agentstore.OpenConsumer(storeRoot, home)
	if err != nil {
		t.Fatalf("a sealed store must open for a consumer: %v", err)
	}
	read, err := consumer.Get(gen.Digest)
	if err != nil {
		t.Fatalf("published generation must be readable: %v", err)
	}
	if read.Digest != gen.Digest || read.TreeManifestSHA256 != gen.TreeManifestSHA256 {
		t.Fatalf("read generation = %+v, published = %+v", read, gen)
	}
	if err := consumer.Verify(gen.Digest); err != nil {
		t.Fatalf("a fresh generation must verify: %v", err)
	}
	assertTemporaryDownloadRemoved(t, tempDir)
}

func TestPublishToStoreRefusesIndexWithoutDigest(t *testing.T) {
	registry := startFakeRegistry(t, publishNoDigest)
	installer, _, _, _ := newStoreInstaller(t, registry.registryURL())

	_, err := installer.PublishToStore(context.Background(), "opencode", t.TempDir())
	if err == nil {
		t.Fatal("an index without a sha256 must not be published into the store")
	}
	if !strings.Contains(err.Error(), "publishes no valid sha256") || !strings.Contains(err.Error(), "verified bytes") {
		t.Fatalf("the refusal must name the missing publication and the store rule, got %v", err)
	}
}

func TestPublishToStoreRefusesMismatchedDigest(t *testing.T) {
	registry := startFakeRegistry(t, publishWrongDigest)
	installer, _, _, _ := newStoreInstaller(t, registry.registryURL())

	_, err := installer.PublishToStore(context.Background(), "opencode", t.TempDir())
	if err == nil {
		t.Fatal("a mismatched digest must not be published into the store")
	}
	if !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("the refusal must state the mismatch, got %v", err)
	}
}

func TestInstallFromStoreWritesReferenceAndVaultRecord(t *testing.T) {
	registry := startFakeRegistry(t, publishRealDigest)
	installer, storage, home, _ := newStoreInstaller(t, registry.registryURL())
	storeRoot := t.TempDir()
	gen, err := installer.PublishToStore(context.Background(), "opencode", storeRoot)
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	sealStore(t, storeRoot)

	if err := installer.InstallFromStore(context.Background(), "opencode", storeRoot, gen.Digest); err != nil {
		t.Fatalf("installing from a sealed verified store must succeed: %v", err)
	}

	entry, err := agentcfg.LoadEntry(storage, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}
	wantLauncher := filepath.Join(storeRoot, "blobs", "sha256", gen.Digest, "opencode")
	if entry.Config.Command != wantLauncher {
		t.Fatalf("command = %q, want the store launcher %q", entry.Config.Command, wantLauncher)
	}
	if len(entry.Config.Args) != 1 || entry.Config.Args[0] != "acp" {
		t.Fatalf("recorded args = %v, want the generation args", entry.Config.Args)
	}
	meta, err := agentcfg.LoadMeta(storage, "opencode")
	if err != nil {
		t.Fatalf("LoadMeta failed: %v", err)
	}
	if meta.Reference == nil {
		t.Fatal("a store-backed install must record the artifact reference")
	}
	if meta.Reference.Store != storeRoot || meta.Reference.Digest != gen.Digest || meta.Reference.TreeManifestSHA256 != gen.TreeManifestSHA256 {
		t.Fatalf("recorded reference = %+v", meta.Reference)
	}
	verification := meta.ArtifactVerification
	if verification == nil || !verification.Verified || verification.Expected != gen.Digest || verification.Actual != gen.Digest {
		t.Fatalf("store-backed install evidence = %+v", verification)
	}

	refs, err := agentstore.ReadReferences(home)
	if err != nil {
		t.Fatalf("ReadReferences failed: %v", err)
	}
	if refs.Agents["opencode"].Digest != gen.Digest {
		t.Fatalf("the home must carry the digest reference, got %+v", refs.Agents)
	}
	if _, err := os.Stat(filepath.Join(home, "agents", "opencode")); !os.IsNotExist(err) {
		t.Fatalf("a store-backed install must not copy software into the home, stat err = %v", err)
	}
}

func TestInstallFromStoreRefusesTamperedGeneration(t *testing.T) {
	registry := startFakeRegistry(t, publishRealDigest)
	installer, storage, home, _ := newStoreInstaller(t, registry.registryURL())
	storeRoot := t.TempDir()
	gen, err := installer.PublishToStore(context.Background(), "opencode", storeRoot)
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	sealStore(t, storeRoot)
	blob := filepath.Join(storeRoot, "blobs", "sha256", gen.Digest, "opencode")
	if err := os.Chmod(blob, 0o644); err != nil {
		t.Fatalf("cannot prepare the tampered generation: %v", err)
	}
	if err := os.WriteFile(blob, []byte("#!/bin/sh\nexec evil\n"), 0o644); err != nil {
		t.Fatalf("cannot tamper the generation: %v", err)
	}

	err = installer.InstallFromStore(context.Background(), "opencode", storeRoot, gen.Digest)
	if err == nil {
		t.Fatal("a tampered generation must be refused")
	}
	if !strings.Contains(err.Error(), "failed verification") {
		t.Fatalf("the refusal must state the failed verification, got %v", err)
	}
	entry, loadErr := agentcfg.LoadEntry(storage, "opencode")
	if loadErr != nil {
		t.Fatalf("LoadEntry failed: %v", loadErr)
	}
	if entry.Config.Command != "" {
		t.Fatalf("a refused store install must not register an agent, got %+v", entry.Config)
	}
	refs, refErr := agentstore.ReadReferences(home)
	if refErr != nil {
		t.Fatalf("ReadReferences failed: %v", refErr)
	}
	if len(refs.Agents) != 0 {
		t.Fatalf("a refused store install must not write a reference, got %+v", refs.Agents)
	}
}

func TestInstallFromStoreRequiresHome(t *testing.T) {
	registry := startFakeRegistry(t, publishRealDigest)
	installer, _, _, _ := newStoreInstaller(t, registry.registryURL())
	installer.home = ""

	err := installer.InstallFromStore(context.Background(), "opencode", t.TempDir(), strings.Repeat("a", sha256HexLength))
	if err == nil || !strings.Contains(err.Error(), "PAL home") {
		t.Fatalf("a home-less installer must refuse instead of writing an unanchored reference, got %v", err)
	}
}

func TestResolveStoreGenerationMatchesRegistryVersion(t *testing.T) {
	registry := startFakeRegistry(t, publishRealDigest)
	installer, _, home, _ := newStoreInstaller(t, registry.registryURL())
	storeRoot := t.TempDir()
	gen, err := installer.PublishToStore(context.Background(), "opencode", storeRoot)
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	sealStore(t, storeRoot)

	resolved, err := installer.ResolveStoreGeneration(context.Background(), "opencode", storeRoot)
	if err != nil {
		t.Fatalf("resolving the only matching generation must succeed: %v", err)
	}
	if resolved.Digest != gen.Digest || resolved.Version != "9.9.9" || resolved.Platform != testPlatform {
		t.Fatalf("resolved generation = %+v", resolved)
	}
	if _, err := os.Stat(filepath.Join(home, "agents", "opencode")); !os.IsNotExist(err) {
		t.Fatalf("resolving a generation must not install anything, stat err = %v", err)
	}
}

func TestResolveStoreGenerationNamesThePublishCommand(t *testing.T) {
	registry := startFakeRegistry(t, publishRealDigest)
	installer, _, home, _ := newStoreInstaller(t, registry.registryURL())
	storeRoot := t.TempDir()
	publisher, err := agentstore.OpenPublisher(storeRoot, home)
	if err != nil {
		t.Fatalf("OpenPublisher failed: %v", err)
	}
	if publisher.Root() != storeRoot {
		t.Fatalf("publisher root = %q", publisher.Root())
	}
	sealStore(t, storeRoot)

	_, err = installer.ResolveStoreGeneration(context.Background(), "opencode", storeRoot)
	if err == nil {
		t.Fatal("an empty store must not resolve a generation")
	}
	for _, want := range []string{"no verified generation", "matrix agent store publish opencode"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error must contain %q, got %v", want, err)
		}
	}
}

func TestUninstallRemovesTheStoreReference(t *testing.T) {
	registry := startFakeRegistry(t, publishRealDigest)
	installer, storage, home, _ := newStoreInstaller(t, registry.registryURL())
	storeRoot := t.TempDir()
	gen, err := installer.PublishToStore(context.Background(), "opencode", storeRoot)
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	sealStore(t, storeRoot)
	if err := installer.InstallFromStore(context.Background(), "opencode", storeRoot, gen.Digest); err != nil {
		t.Fatalf("InstallFromStore failed: %v", err)
	}
	if err := installer.Uninstall(context.Background(), "opencode"); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	refs, err := agentstore.ReadReferences(home)
	if err != nil {
		t.Fatalf("ReadReferences failed: %v", err)
	}
	if len(refs.Agents) != 0 {
		t.Fatalf("uninstall must drop the reference that kept the generation alive, got %+v", refs.Agents)
	}
	if _, err := agentcfg.LoadEntry(storage, "opencode"); err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}
}
