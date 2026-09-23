package agentmgr

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/middleware"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
	networkprovider "github.com/Josepavese/matrix/internal/providers/network"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// testPlatform is the registry platform key the fixture installs for. The
// client platform is injected instead of inherited so the three integrity
// outcomes are exercised identically on every host and on CI.
const testPlatform = "linux-x86_64"

// digestPublication selects what the synthetic index advertises for the
// artifact it serves.
type digestPublication int

const (
	publishRealDigest digestPublication = iota
	publishWrongDigest
	publishNoDigest
	publishMalformedDigest
)

// fakeRegistry serves a registry index and one real .tar.gz artifact over
// loopback: the install path runs against real HTTP, a real download, a real
// hash and a real extraction, without touching the public network.
type fakeRegistry struct {
	server   *httptest.Server
	artifact []byte
	digest   string
}

func startFakeRegistry(t *testing.T, publication digestPublication) *fakeRegistry {
	t.Helper()
	return startFakeRegistryEntry(t, publication, "./opencode", map[string]string{"opencode": "#!/bin/sh\nexec opencode-acp\n"})
}

// startFakeRegistryEntry is startFakeRegistry with the launcher path and the
// archive contents under the test's control, so the command gate is exercised
// against the same real HTTP download, hash and extraction path as production.
func startFakeRegistryEntry(t *testing.T, publication digestPublication, cmd string, files map[string]string) *fakeRegistry {
	t.Helper()

	artifact := testArtifactFiles(t, files)
	digest := sha256Hex(artifact)
	registry := &fakeRegistry{artifact: artifact, digest: digest}

	published := map[digestPublication]string{
		publishRealDigest:      digest,
		publishWrongDigest:     strings.Repeat("0", sha256HexLength),
		publishNoDigest:        "",
		publishMalformedDigest: "sha256:" + digest,
	}[publication]

	mux := http.NewServeMux()
	mux.HandleFunc("/registry.json", func(w http.ResponseWriter, r *http.Request) {
		// The artifact URL is derived from the request host so the index and
		// the download always share the loopback base URL.
		entry := map[string]any{
			"archive": "http://" + r.Host + "/artifact.tar.gz",
			"cmd":     cmd,
			"args":    []string{"acp"},
		}
		if publication != publishNoDigest {
			entry["sha256"] = published
		}
		index := map[string]any{
			"version": "1.0.0",
			"agents": []any{map[string]any{
				"id":      "opencode",
				"name":    "OpenCode",
				"version": "9.9.9",
				"distribution": map[string]any{
					"binary": map[string]any{testPlatform: entry},
				},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(index)
	})
	mux.HandleFunc("/artifact.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(registry.artifact)
	})

	registry.server = httptest.NewServer(mux)
	t.Cleanup(registry.server.Close)
	return registry
}

func (r *fakeRegistry) registryURL() string {
	return r.server.URL + "/registry.json"
}

// artifactURL is the archive URL the served index advertises, so a test can
// assert that a refusal or a warning names the artifact.
func (r *fakeRegistry) artifactURL() string {
	return r.server.URL + "/artifact.tar.gz"
}

// newTestInstaller wires the production providers against an isolated vault,
// install directory and temporary directory.
func newTestInstaller(t *testing.T, registryURL string) (*Installer, middleware.Storage, string, string) {
	t.Helper()

	// The artifact is downloaded into the OS temporary directory: isolating it
	// makes "the temporary download is removed" an assertion, and keeps the
	// workstation clean.
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)

	store := memstore.New()
	baseDir := t.TempDir()

	netProvider := networkprovider.NewProvider()
	client := NewRegistryClient(netProvider, registryURL)
	client.goos = "linux"
	client.arch = "amd64"

	installer, err := NewInstaller(InstallerConfig{
		Net:      netProvider,
		Archive:  osfs.NewArchiveProvider(),
		Storage:  store,
		FS:       osfs.NewFSProvider(),
		Process:  execprovider.NewProvider(),
		Registry: client,
		BaseDir:  baseDir,
	})
	if err != nil {
		t.Fatalf("NewInstaller failed: %v", err)
	}
	return installer, store, baseDir, tempDir
}

func TestInstallVerifiesPublishedArtifactDigest(t *testing.T) {
	registry := startFakeRegistry(t, publishRealDigest)
	installer, store, baseDir, tempDir := newTestInstaller(t, registry.registryURL())

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("a matching digest must install, got %v", err)
	}

	meta, err := agentcfg.LoadMeta(store, "opencode")
	if err != nil {
		t.Fatalf("LoadMeta failed: %v", err)
	}
	verification := meta.ArtifactVerification
	if verification == nil {
		t.Fatal("the install must record the integrity evidence")
	}
	if !verification.Verified || verification.Status != agentcfg.ArtifactVerified {
		t.Fatalf("expected a verified artifact, got %+v", verification)
	}
	if verification.Expected != registry.digest || verification.Actual != registry.digest {
		t.Fatalf("expected and actual digests must both be the published one, got %+v", verification)
	}
	if verification.Platform != testPlatform {
		t.Fatalf("platform = %q, want %q", verification.Platform, testPlatform)
	}

	entry, err := agentcfg.LoadEntry(store, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}
	if entry.Config.Command != filepath.Join(baseDir, "opencode", "opencode") {
		t.Fatalf("installed command = %q", entry.Config.Command)
	}
	if _, err := os.Stat(entry.Config.Command); err != nil {
		t.Fatalf("the verified artifact must be extracted, stat failed: %v", err)
	}
	assertTemporaryDownloadRemoved(t, tempDir)
}

func TestInstallRefusesArtifactWithMismatchedDigest(t *testing.T) {
	registry := startFakeRegistry(t, publishWrongDigest)
	installer, store, baseDir, tempDir := newTestInstaller(t, registry.registryURL())

	err := installer.Install(context.Background(), "opencode")
	if err == nil {
		t.Fatal("a mismatched digest must refuse the installation")
	}
	t.Logf("refused install: %v", err)
	if !strings.Contains(err.Error(), "sha256 mismatch") || !strings.Contains(err.Error(), "refusing to install") {
		t.Fatalf("the error must state the mismatch and the refusal, got %v", err)
	}
	if !strings.Contains(err.Error(), registry.digest) {
		t.Fatalf("the error must name the digest of the downloaded bytes, got %v", err)
	}
	assertNoHalfInstallation(t, baseDir, tempDir, store)
}

func TestInstallRefusesMalformedPublishedDigest(t *testing.T) {
	registry := startFakeRegistry(t, publishMalformedDigest)
	installer, store, baseDir, tempDir := newTestInstaller(t, registry.registryURL())

	err := installer.Install(context.Background(), "opencode")
	if err == nil {
		t.Fatal("a published digest that is not a sha256 must refuse the installation")
	}
	if !strings.Contains(err.Error(), "malformed sha256") {
		t.Fatalf("the error must name the malformed digest, got %v", err)
	}
	assertNoHalfInstallation(t, baseDir, tempDir, store)
}

// TestInstallRefusesArtifactWithoutPublishedDigest is the fail-closed default:
// when the index publishes no sha256 there is nothing to compare the download
// against, so the install stops and says which artifact is missing what. It
// replaces the earlier contract, where this case installed silently.
func TestInstallRefusesArtifactWithoutPublishedDigest(t *testing.T) {
	registry := startFakeRegistry(t, publishNoDigest)
	installer, store, baseDir, tempDir := newTestInstaller(t, registry.registryURL())

	err := installer.Install(context.Background(), "opencode")
	if err == nil {
		t.Fatal("an artifact whose registry entry publishes no digest must refuse the installation by default")
	}
	t.Logf("refused install: %v", err)
	for _, want := range []string{"publishes no sha256", registry.artifactURL(), testPlatform, "opencode", "refusing to install", "--allow-unverified"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal must contain %q, got %v", want, err)
		}
	}
	assertNoHalfInstallation(t, baseDir, tempDir, store)
}

// TestInstallAllowUnverifiedOptsIntoAnArtifactWithoutPublishedDigest pins the
// documented opt-in: the operator can accept an unverified artifact, the
// override is printed with the artifact it applies to, and the evidence keeps
// saying nothing was verified.
func TestInstallAllowUnverifiedOptsIntoAnArtifactWithoutPublishedDigest(t *testing.T) {
	registry := startFakeRegistry(t, publishNoDigest)
	installer, store, _, tempDir := newTestInstaller(t, registry.registryURL())

	var progress bytes.Buffer
	installer.SetProgressWriter(&progress)
	installer.SetAllowUnverified(true)

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("--allow-unverified must let the install proceed, got %v", err)
	}

	out := progress.String()
	for _, want := range []string{"--allow-unverified", registry.artifactURL(), testPlatform} {
		if !strings.Contains(out, want) {
			t.Fatalf("the override must be visible with %q, got:\n%s", want, out)
		}
	}

	meta, err := agentcfg.LoadMeta(store, "opencode")
	if err != nil {
		t.Fatalf("LoadMeta failed: %v", err)
	}
	verification := meta.ArtifactVerification
	if verification == nil {
		t.Fatal("the install must record why nothing was verified")
	}
	if verification.Verified {
		t.Fatalf("an unpublished digest must never read as verified, got %+v", verification)
	}
	if verification.Status != agentcfg.ArtifactDigestNotPublished {
		t.Fatalf("status = %q, want %q", verification.Status, agentcfg.ArtifactDigestNotPublished)
	}
	if verification.Override != agentcfg.ArtifactOverrideAllowUnverified {
		t.Fatalf("override = %q, want %q", verification.Override, agentcfg.ArtifactOverrideAllowUnverified)
	}
	if verification.Expected != "" {
		t.Fatalf("no published digest exists, got %q", verification.Expected)
	}
	if verification.Actual != registry.digest {
		t.Fatalf("the digest of the downloaded artifact must still be recorded, got %q", verification.Actual)
	}

	entry, err := agentcfg.LoadEntry(store, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}
	if _, err := os.Stat(entry.Config.Command); err != nil {
		t.Fatalf("the artifact must still be extracted, stat failed: %v", err)
	}
	assertTemporaryDownloadRemoved(t, tempDir)
}

// TestAllowUnverifiedDoesNotAcceptADigestMismatch keeps the opt-in narrow: it
// answers "the index publishes nothing", not "the index publishes something the
// artifact does not match". A mismatch and a malformed digest stay refusals.
func TestAllowUnverifiedDoesNotAcceptADigestMismatch(t *testing.T) {
	for _, publication := range []digestPublication{publishWrongDigest, publishMalformedDigest} {
		registry := startFakeRegistry(t, publication)
		installer, store, baseDir, tempDir := newTestInstaller(t, registry.registryURL())
		installer.SetAllowUnverified(true)

		err := installer.Install(context.Background(), "opencode")
		if err == nil {
			t.Fatal("--allow-unverified must not accept an artifact whose published digest does not match")
		}
		if !strings.Contains(err.Error(), "refusing to install") {
			t.Fatalf("the refusal must keep its reason, got %v", err)
		}
		assertNoHalfInstallation(t, baseDir, tempDir, store)
	}
}

// assertNoHalfInstallation checks that a refused install left nothing behind:
// no agent directory, no vault entry, no metadata and no temporary download.
func assertNoHalfInstallation(t *testing.T, baseDir, tempDir string, store middleware.Storage) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(baseDir, "opencode")); !os.IsNotExist(err) {
		t.Fatalf("a refused artifact must not create the agent directory, stat error = %v", err)
	}

	entry, err := agentcfg.LoadEntry(store, "opencode")
	if err != nil {
		t.Fatalf("LoadEntry failed: %v", err)
	}
	if entry.Config.Command != "" {
		t.Fatalf("a refused artifact must not be registered, got %+v", entry.Config)
	}

	meta, err := agentcfg.LoadMeta(store, "opencode")
	if err != nil {
		t.Fatalf("LoadMeta failed: %v", err)
	}
	if meta.ArtifactVerification != nil {
		t.Fatalf("a refused artifact must not record evidence, got %+v", meta.ArtifactVerification)
	}

	assertTemporaryDownloadRemoved(t, tempDir)
}

func assertTemporaryDownloadRemoved(t *testing.T, tempDir string) {
	t.Helper()

	leftovers, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(leftovers) != 0 {
		names := make([]string, 0, len(leftovers))
		for _, entry := range leftovers {
			names = append(names, entry.Name())
		}
		t.Fatalf("the temporary download must be removed, found %v", names)
	}
}

// testArtifactFiles builds a real .tar.gz holding the given files in a stable
// order, so the install path runs the same extraction code as a production
// download and a command-resolution test extracts the layout it asserts on.
func testArtifactFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, name := range names {
		payload := []byte(files[name])
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: name, Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("tar header failed: %v", err)
		}
		if _, err := tarWriter.Write(payload); err != nil {
			t.Fatalf("tar body failed: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("closing the tar writer failed: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("closing the gzip writer failed: %v", err)
	}
	return buffer.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
