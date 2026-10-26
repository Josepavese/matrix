package agentstore

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublishArchiveHappyPath(t *testing.T) {
	store := newTestPublisher(t)
	published, digest, archive := publishTarGz(t, store, launcherEntries())

	if published.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %d, want %d", published.SchemaVersion, SchemaVersion)
	}
	if published.Digest != digest {
		t.Fatalf("digest = %s, want %s", published.Digest, digest)
	}
	if published.TreeManifestSHA256 == "" || len(published.TreeManifestSHA256) != 64 {
		t.Fatalf("tree manifest digest = %q", published.TreeManifestSHA256)
	}
	if published.PublishedAt.IsZero() || published.PublishedAt.Location() != time.UTC {
		t.Fatalf("published_at = %v, want a UTC timestamp", published.PublishedAt)
	}
	if published.Artifact != "artifact.tar.gz" {
		t.Fatalf("artifact was not preserved: %q", published.Artifact)
	}

	blobDir, err := store.BlobDir(digest)
	if err != nil {
		t.Fatalf("blob dir: %v", err)
	}
	launcher, err := os.Stat(filepath.Join(blobDir, "opencode"))
	if err != nil {
		t.Fatalf("stat launcher: %v", err)
	}
	if !supportsUnixPermissions() {
		return
	}
	if launcher.Mode().Perm()&0o111 == 0 {
		t.Fatalf("launcher lost its execute bit: %04o", launcher.Mode().Perm())
	}
	assertSealedTree(t, blobDir)

	for _, metadata := range []string{store.generationPath(digest), store.manifestPath(digest)} {
		info, err := os.Stat(metadata)
		if err != nil {
			t.Fatalf("stat metadata %s: %v", metadata, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("metadata %s mode = %04o, want 0600", metadata, info.Mode().Perm())
		}
	}
	if err := store.Verify(digest); err != nil {
		t.Fatalf("verify published generation: %v", err)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("source archive disappeared: %v", err)
	}
}

func TestPublishArchivePreservesLauncherVerbatim(t *testing.T) {
	store := newTestPublisher(t)
	archive := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeTestTarGz(t, archive, launcherEntries())
	digest := digestOfFile(t, archive)
	generation := sampleGeneration()
	generation.Launcher = "./opencode"
	published, err := store.PublishArchive(testArchiver(), archive, digest, generation)
	if err != nil {
		t.Fatalf("publish with a registry-style launcher: %v", err)
	}
	if published.Launcher != "./opencode" {
		t.Fatalf("launcher = %q, want the caller's ./opencode", published.Launcher)
	}
	blobDir, err := store.BlobDir(published.Digest)
	if err != nil {
		t.Fatalf("blob dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(blobDir, filepath.FromSlash(published.Launcher))); err != nil {
		t.Fatalf("published launcher does not resolve inside the tree: %v", err)
	}
	if err := store.Verify(published.Digest); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestPublishArchiveIsIdempotent(t *testing.T) {
	store := newTestPublisher(t)
	archive := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeTestTarGz(t, archive, launcherEntries())
	digest := digestOfFile(t, archive)

	first, err := store.PublishArchive(testArchiver(), archive, digest, sampleGeneration())
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	second, err := store.PublishArchive(testArchiver(), archive, digest, sampleGeneration())
	if err != nil {
		t.Fatalf("idempotent republish: %v", err)
	}
	if second.Digest != first.Digest || !second.PublishedAt.Equal(first.PublishedAt) {
		t.Fatalf("republish returned %+v, want %+v", second, first)
	}
	list, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("generations = %d, want 1", len(list))
	}
}

func TestPublishArchiveRejectsImmutabilityConflict(t *testing.T) {
	store := newTestPublisher(t)
	archive := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeTestTarGz(t, archive, launcherEntries())
	digest := digestOfFile(t, archive)
	if _, err := store.PublishArchive(testArchiver(), archive, digest, sampleGeneration()); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	conflicts := []struct {
		name   string
		change func(*Generation)
		field  string
	}{
		{"version", func(g *Generation) { g.Version = "9.9.9" }, "version"},
		{"agent_id", func(g *Generation) { g.AgentID = "other" }, "agent_id"},
		{"platform", func(g *Generation) { g.Platform = "darwin/arm64" }, "platform"},
		{"artifact", func(g *Generation) { g.Artifact = "other.tar.gz" }, "artifact"},
		{"launcher", func(g *Generation) { g.Launcher = "lib/data.txt" }, "launcher"},
	}
	for _, conflict := range conflicts {
		t.Run(conflict.name, func(t *testing.T) {
			incoming := sampleGeneration()
			conflict.change(&incoming)
			_, err := store.PublishArchive(testArchiver(), archive, digest, incoming)
			if err == nil {
				t.Fatalf("expected an immutability error for %s", conflict.name)
			}
			for _, fragment := range []string{"immutable", digest, conflict.field} {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error %q does not name %q", err, fragment)
				}
			}
		})
	}
}

func TestPublishArchiveRefusesTamperedExistingGeneration(t *testing.T) {
	store := newTestPublisher(t)
	_, digest, archive := publishTarGz(t, store, launcherEntries())
	blobDir, err := store.BlobDir(digest)
	if err != nil {
		t.Fatalf("blob dir: %v", err)
	}
	unlockForTest(t, blobDir)
	if err := os.WriteFile(filepath.Join(blobDir, "opencode"), []byte("#!/bin/sh\nexec evil\n"), 0o755); err != nil {
		t.Fatalf("tamper launcher: %v", err)
	}
	relockForTest(t, blobDir)
	if _, err := store.PublishArchive(testArchiver(), archive, digest, sampleGeneration()); err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("a tampered existing generation must not be reused: %v", err)
	}
}

func TestPublishArchiveRejectsDigestMismatch(t *testing.T) {
	store := newTestPublisher(t)
	archive := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeTestTarGz(t, archive, launcherEntries())
	wrong := hexDigest("0")
	if _, err := store.PublishArchive(testArchiver(), archive, wrong, sampleGeneration()); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected a digest mismatch refusal: %v", err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("a mismatched archive published %d generations", len(list))
	}
}

func TestRejectUnsupportedEntriesRejectsSymlink(t *testing.T) {
	if !supportsUnixPermissions() {
		t.Skip("symlinks are required")
	}
	tree := t.TempDir()
	writeFile(t, filepath.Join(tree, "opencode"), "#!/bin/sh\n", 0o755)
	if err := os.Symlink("opencode", filepath.Join(tree, "escape")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	err := rejectUnsupportedEntries(tree)
	if err == nil || !strings.Contains(err.Error(), "symlinks and special files") {
		t.Fatalf("expected the post-extraction walk to refuse the symlink: %v", err)
	}
}

func TestPublishArchiveRejectsSymlinkLauncher(t *testing.T) {
	store := newTestPublisher(t)
	entries := []archiveEntry{
		{name: "lib/data.txt", mode: 0o644, body: "data"},
		{name: "opencode", kind: tar.TypeSymlink, link: "lib/data.txt"},
	}
	archive := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeTestTarGz(t, archive, entries)
	digest := digestOfFile(t, archive)
	_, err := store.PublishArchive(testArchiver(), archive, digest, sampleGeneration())
	if err == nil || !strings.Contains(err.Error(), "not found in artifact") {
		t.Fatalf("a dropped symlink launcher must leave the generation unusable: %v", err)
	}
	assertNoStagingLeftovers(t, store)
}

func TestPublishArchiveRejectsHardlinkLauncher(t *testing.T) {
	store := newTestPublisher(t)
	entries := []archiveEntry{
		{name: "lib/data.txt", mode: 0o644, body: "data"},
		{name: "opencode", kind: tar.TypeLink, link: "lib/data.txt"},
	}
	archive := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeTestTarGz(t, archive, entries)
	digest := digestOfFile(t, archive)
	if _, err := store.PublishArchive(testArchiver(), archive, digest, sampleGeneration()); err == nil {
		t.Fatal("expected the hardlink launcher to be refused")
	}
	assertNoStagingLeftovers(t, store)
}

func TestPublishArchiveDropsUnsafeArchiveEntries(t *testing.T) {
	store := newTestPublisher(t)
	entries := []archiveEntry{
		{name: "opencode", mode: 0o755, body: "#!/bin/sh\n"},
		{name: "/etc/evil-agentstore-test", mode: 0o644, body: "nope"},
		{name: "escape", kind: tar.TypeSymlink, link: "opencode"},
	}
	archive := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeTestTarGz(t, archive, entries)
	digest := digestOfFile(t, archive)
	published, err := store.PublishArchive(testArchiver(), archive, digest, sampleGeneration())
	if err != nil {
		t.Fatalf("the provider drops hostile entries; the remaining tree is publishable: %v", err)
	}
	blobDir, err := store.BlobDir(published.Digest)
	if err != nil {
		t.Fatalf("blob dir: %v", err)
	}
	for _, unsafe := range []string{"escape", "evil-agentstore-test"} {
		if _, err := os.Lstat(filepath.Join(blobDir, unsafe)); !os.IsNotExist(err) {
			t.Fatalf("unsafe archive entry %q reached the published tree: %v", unsafe, err)
		}
	}
	if _, err := os.Stat("/etc/evil-agentstore-test"); err == nil {
		t.Fatal("absolute archive entry escaped the staging directory")
	}
	if err := store.Verify(published.Digest); err != nil {
		t.Fatalf("verify after dropping unsafe entries: %v", err)
	}
}

func TestPublishArchiveRejectsNilProvider(t *testing.T) {
	store := newTestPublisher(t)
	archive := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeTestTarGz(t, archive, launcherEntries())
	digest := digestOfFile(t, archive)
	if _, err := store.PublishArchive(nil, archive, digest, sampleGeneration()); err == nil || !strings.Contains(err.Error(), "archive provider is required") {
		t.Fatalf("expected a missing provider refusal: %v", err)
	}
}

func TestPublishArchiveRejectsLauncherMissing(t *testing.T) {
	store := newTestPublisher(t)
	archive := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeTestTarGz(t, archive, []archiveEntry{{name: "lib/data.txt", mode: 0o644, body: "data"}})
	digest := digestOfFile(t, archive)
	generation := sampleGeneration()
	generation.Launcher = "missing-binary"
	if _, err := store.PublishArchive(testArchiver(), archive, digest, generation); err == nil || !strings.Contains(err.Error(), "not found in artifact") {
		t.Fatalf("expected a missing launcher refusal: %v", err)
	}
	assertNoStagingLeftovers(t, store)
}

func TestPublishArchiveRejectsEscapingLauncher(t *testing.T) {
	store := newTestPublisher(t)
	archive := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeTestTarGz(t, archive, launcherEntries())
	digest := digestOfFile(t, archive)
	for _, launcher := range []string{"../outside", "lib/../../outside", "/bin/sh", "."} {
		generation := sampleGeneration()
		generation.Launcher = launcher
		if _, err := store.PublishArchive(testArchiver(), archive, digest, generation); err == nil {
			t.Fatalf("expected launcher %q to be refused", launcher)
		}
	}
}

func TestPublishArchiveRejectsUnsupportedArchiveFormat(t *testing.T) {
	store := newTestPublisher(t)
	archive := filepath.Join(t.TempDir(), "artifact.tar.xz")
	if err := os.WriteFile(archive, []byte("not really xz"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	digest := digestOfFile(t, archive)
	if _, err := store.PublishArchive(testArchiver(), archive, digest, sampleGeneration()); err == nil || !strings.Contains(err.Error(), "unsupported archive format") {
		t.Fatalf("expected an unsupported format refusal: %v", err)
	}
}

func TestPublishArchiveFromZip(t *testing.T) {
	store := newTestPublisher(t)
	archive := filepath.Join(t.TempDir(), "artifact.zip")
	writeTestZip(t, archive, launcherEntries())
	digest := digestOfFile(t, archive)
	published, err := store.PublishArchive(testArchiver(), archive, digest, sampleGeneration())
	if err != nil {
		t.Fatalf("publish zip: %v", err)
	}
	if published.Digest != digest {
		t.Fatalf("digest = %s, want %s", published.Digest, digest)
	}
	if err := store.Verify(digest); err != nil {
		t.Fatalf("verify zip generation: %v", err)
	}
}

func TestPublishSecondGenerationKeepsFirst(t *testing.T) {
	store := newTestPublisher(t)
	_, firstDigest, _ := publishTarGz(t, store, launcherEntries())
	secondEntries := launcherEntries()
	secondEntries[1].body = "second artifact data"
	_, secondDigest, _ := publishTarGz(t, store, secondEntries)
	if firstDigest == secondDigest {
		t.Fatal("two different archives produced the same digest")
	}
	for _, digest := range []string{firstDigest, secondDigest} {
		if err := store.Verify(digest); err != nil {
			t.Fatalf("verify %s: %v", digest, err)
		}
	}
	list, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Digest > list[1].Digest {
		t.Fatalf("list = %+v, want two generations sorted by digest", list)
	}
}

func assertNoStagingLeftovers(t *testing.T, store *Store) {
	t.Helper()
	entries, err := os.ReadDir(store.tmpDir())
	if err != nil {
		t.Fatalf("read staging: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("%d staging directories left behind", len(entries))
	}
}
