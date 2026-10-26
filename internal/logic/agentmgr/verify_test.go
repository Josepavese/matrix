package agentmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentstore"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// storeFixture is a real published store generation, its sealed store root and
// the reference a home records after installing from it.
type storeFixture struct {
	root       string
	home       string
	digest     string
	generation agentstore.Generation
}

// publishStoreFixture publishes one real artifact (a launcher and a data file)
// into a temporary store. The store lives outside the fixture home, and sealing
// it turns it into the read-only shape a consumer accepts.
func publishStoreFixture(t *testing.T, agentID, version string) storeFixture {
	t.Helper()

	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "agent-store")
	publisher, err := agentstore.OpenPublisher(root, home)
	if err != nil {
		t.Fatalf("OpenPublisher: %v", err)
	}
	archive := testArtifactFiles(t, map[string]string{
		"bin/agent-bin": "#!/bin/sh\nexit 0\n",
		"share/data":    "fixture-data\n",
	})
	digest := sha256Hex(archive)
	archivePath := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write fixture archive: %v", err)
	}
	generation, err := publisher.PublishArchive(osfs.NewArchiveProvider(), archivePath, digest, agentstore.Generation{
		AgentID: agentID, Version: version, Platform: testPlatform,
		Artifact: "https://example.invalid/artifact.tar.gz", Launcher: "bin/agent-bin",
		Args: []string{"--serve"}, License: "MIT", Repository: "example/repo",
	})
	if err != nil {
		t.Fatalf("PublishArchive: %v", err)
	}
	sealStoreLayout(t, root)
	return storeFixture{root: root, home: home, digest: digest, generation: generation}
}

func (f storeFixture) reference() *agentcfg.ArtifactReference {
	return &agentcfg.ArtifactReference{
		Store: f.root, Digest: f.digest, TreeManifestSHA256: f.generation.TreeManifestSHA256,
		Launcher: f.generation.Launcher, Version: f.generation.Version, Platform: f.generation.Platform,
	}
}

func (f storeFixture) blobDir() string {
	return filepath.Join(f.root, "blobs", "sha256", f.digest)
}

func (f storeFixture) launcherPath() string {
	return filepath.Join(f.blobDir(), filepath.FromSlash(f.generation.Launcher))
}

// sealStore makes the store layout read-only for the current user and registers
// the cleanup that restores write permission so t.TempDir can remove it.
func sealStoreLayout(t *testing.T, root string) {
	t.Helper()
	for _, dir := range []string{root, filepath.Join(root, "blobs"), filepath.Join(root, "blobs", "sha256"), filepath.Join(root, "generations")} {
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatalf("seal store directory %s: %v", dir, err)
		}
	}
	t.Cleanup(func() { makeStoreTreeWritable(root) })
}

func makeStoreTreeWritable(root string) {
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry == nil {
			return nil
		}
		if entry.IsDir() {
			_ = os.Chmod(path, 0o755)
		}
		return nil
	})
}

func savedStoreBackedAgent(t *testing.T, storage middleware.Storage, fixture storeFixture) {
	t.Helper()
	saveAgentEntry(t, storage, fixture.generation.AgentID, agentcfg.Config{
		Command: fixture.launcherPath(), Args: fixture.generation.Args, Kind: "acp", Transport: "stdio",
	})
	saveAgentMeta(t, storage, fixture.generation.AgentID, agentcfg.Meta{
		ID: fixture.generation.AgentID, Version: fixture.generation.Version,
		Reference: fixture.reference(),
	})
}

func TestVerifyInstalledProvesAStoreGenerationTree(t *testing.T) {
	installer, storage, _, _ := newReadSideInstaller(t, "")
	fixture := publishStoreFixture(t, "stored", "2.5.0")
	savedStoreBackedAgent(t, storage, fixture)

	results, err := installer.VerifyInstalled([]string{"stored"})
	if err != nil {
		t.Fatalf("VerifyInstalled: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	result := results[0]
	if result.Method != VerifyMethodStoreTree || !result.Verified {
		t.Fatalf("store-backed verification = %+v, want a verified store_tree", result)
	}
	if !strings.Contains(result.Detail, fixture.digest) {
		t.Fatalf("detail %q does not name the verified generation %s", result.Detail, fixture.digest)
	}
}

func TestVerifyInstalledRefusesATamperedStoreGeneration(t *testing.T) {
	installer, storage, _, _ := newReadSideInstaller(t, "")
	fixture := publishStoreFixture(t, "stored", "2.5.0")
	savedStoreBackedAgent(t, storage, fixture)

	makeStoreTreeWritable(fixture.blobDir())
	if err := os.Chmod(fixture.launcherPath(), 0o644); err != nil {
		t.Fatalf("unseal the published launcher: %v", err)
	}
	if err := os.WriteFile(fixture.launcherPath(), []byte("#!/bin/sh\nexit 1\n"), 0o644); err != nil {
		t.Fatalf("tamper with the published launcher: %v", err)
	}
	sealStoreLayout(t, fixture.root)

	results, err := installer.VerifyInstalled([]string{"stored"})
	if err != nil {
		t.Fatalf("VerifyInstalled: %v", err)
	}
	if len(results) != 1 || results[0].Verified {
		t.Fatalf("a tampered generation must not verify: %+v", results)
	}
	if results[0].Method != VerifyMethodStoreTree {
		t.Fatalf("method = %q, want %q", results[0].Method, VerifyMethodStoreTree)
	}
}

func TestVerifyInstalledRefusesAWritableStore(t *testing.T) {
	installer, storage, _, _ := newReadSideInstaller(t, "")
	fixture := publishStoreFixture(t, "stored", "2.5.0")
	savedStoreBackedAgent(t, storage, fixture)
	// A writable store is not a consumer trust boundary: the runtime could swap
	// the bytes after verification.
	for _, dir := range []string{fixture.root, filepath.Join(fixture.root, "blobs"), filepath.Join(fixture.root, "blobs", "sha256"), filepath.Join(fixture.root, "generations")} {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatalf("unseal %s: %v", dir, err)
		}
	}

	results, err := installer.VerifyInstalled([]string{"stored"})
	if err != nil {
		t.Fatalf("VerifyInstalled: %v", err)
	}
	if len(results) != 1 || results[0].Verified {
		t.Fatalf("a writable store must fail closed: %+v", results)
	}
}

func TestVerifyInstalledReportsRecordedEvidenceWithoutClaimingBytesProven(t *testing.T) {
	installer, storage, _, _ := newReadSideInstaller(t, "")
	launcher := filepath.Join(t.TempDir(), "agent-bin")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write launcher: %v", err)
	}
	saveAgentEntry(t, storage, "local", agentcfg.Config{Command: launcher, Kind: "acp", Transport: "stdio"})
	saveAgentMeta(t, storage, "local", agentcfg.Meta{
		ID: "local", Version: "1.0.0", DistTypes: []string{"binary"},
		ArtifactVerification: &agentcfg.ArtifactVerification{Verified: true, Status: agentcfg.ArtifactVerified, Platform: testPlatform},
	})

	results, err := installer.VerifyInstalled([]string{"local"})
	if err != nil {
		t.Fatalf("VerifyInstalled: %v", err)
	}
	result := results[0]
	if result.Method != VerifyMethodRecordedEvidence || !result.Verified {
		t.Fatalf("per-home verification = %+v, want a verified recorded_evidence", result)
	}
	if !strings.Contains(result.Detail, "not re-proven offline") {
		t.Fatalf("detail must say the bytes were not re-proven offline: %q", result.Detail)
	}
}

func TestVerifyInstalledRejectsMissingLauncherAndUnverifiedEvidence(t *testing.T) {
	installer, storage, _, _ := newReadSideInstaller(t, "")
	saveAgentEntry(t, storage, "gone", agentcfg.Config{Command: filepath.Join(t.TempDir(), "missing-binary")})
	saveAgentMeta(t, storage, "gone", agentcfg.Meta{
		ID: "gone", DistTypes: []string{"binary"},
		ArtifactVerification: &agentcfg.ArtifactVerification{Verified: true, Status: agentcfg.ArtifactVerified, Platform: testPlatform},
	})
	saveAgentEntry(t, storage, "unverified", agentcfg.Config{Command: filepath.Join(t.TempDir(), "unverified-binary")})
	saveAgentMeta(t, storage, "unverified", agentcfg.Meta{
		ID: "unverified", DistTypes: []string{"binary"},
		ArtifactVerification: &agentcfg.ArtifactVerification{Verified: false, Status: agentcfg.ArtifactDigestNotPublished, Platform: testPlatform},
	})

	results, err := installer.VerifyInstalled([]string{"gone", "unverified"})
	if err != nil {
		t.Fatalf("VerifyInstalled: %v", err)
	}
	byID := map[string]VerifyResult{}
	for _, result := range results {
		byID[result.ID] = result
	}
	if gone := byID["gone"]; gone.Verified || !strings.Contains(gone.Detail, "missing") {
		t.Fatalf("missing launcher result = %+v", gone)
	}
	if unverified := byID["unverified"]; unverified.Verified || !strings.Contains(unverified.Detail, "not verified") {
		t.Fatalf("unverified evidence result = %+v", unverified)
	}
}

func TestVerifyInstalledReportsUnknownAgentsWithoutFailingTheBatch(t *testing.T) {
	installer, storage, _, _ := newReadSideInstaller(t, "")
	launcher := filepath.Join(t.TempDir(), "agent-bin")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write launcher: %v", err)
	}
	saveAgentEntry(t, storage, "zeta", agentcfg.Config{Command: launcher})
	saveAgentMeta(t, storage, "zeta", agentcfg.Meta{
		ID: "zeta", DistTypes: []string{"binary"},
		ArtifactVerification: &agentcfg.ArtifactVerification{Verified: true, Status: agentcfg.ArtifactVerified, Platform: testPlatform},
	})

	results, err := installer.VerifyInstalled([]string{"ghost", "zeta", "zeta"})
	if err != nil {
		t.Fatalf("VerifyInstalled: %v", err)
	}
	if len(results) != 2 || results[0].ID != "ghost" || results[1].ID != "zeta" {
		t.Fatalf("results are not deduplicated and sorted: %+v", results)
	}
	if results[0].Verified || results[0].Method == "" || !strings.Contains(results[0].Detail, "not registered") {
		t.Fatalf("unknown agent result = %+v", results[0])
	}

	all, err := installer.VerifyInstalled(nil)
	if err != nil {
		t.Fatalf("VerifyInstalled all: %v", err)
	}
	if len(all) != 1 || all[0].ID != "zeta" {
		t.Fatalf("all-agents verification = %+v", all)
	}
}
