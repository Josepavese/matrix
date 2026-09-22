package agentmgr

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
	networkprovider "github.com/Josepavese/matrix/internal/providers/network"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// TestInstallVerifiesRealRegistryArtifactOptIn is the only test in this package
// that needs the public network, so it stays behind an explicit switch and never
// runs with the default `go test ./...`:
//
//	MATRIX_REGISTRY_INTEGRITY_REAL=1 go test ./internal/logic/agentmgr/ \
//	    -run TestInstallVerifiesRealRegistryArtifactOptIn -count=1 -v -timeout 20m
//
// It installs the default agent from the real ACP registry into a throwaway
// directory and asserts that the digest the index publishes for this platform is
// the digest of the artifact Matrix downloaded.
func TestInstallVerifiesRealRegistryArtifactOptIn(t *testing.T) {
	if os.Getenv("MATRIX_REGISTRY_INTEGRITY_REAL") != "1" {
		t.Skip("set MATRIX_REGISTRY_INTEGRITY_REAL=1 to download the published artifact from the real ACP registry")
	}

	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)

	store := memstore.New()
	netProvider := networkprovider.NewProvider()
	installer, err := NewInstaller(InstallerConfig{
		Net:      netProvider,
		Archive:  osfs.NewArchiveProvider(),
		Storage:  store,
		FS:       osfs.NewFSProvider(),
		Process:  execprovider.NewProvider(),
		Registry: NewRegistryClient(netProvider, ""),
		BaseDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewInstaller failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := installer.Install(ctx, "opencode"); err != nil {
		t.Fatalf("installing opencode from the real registry failed: %v", err)
	}

	meta, err := agentcfg.LoadMeta(store, "opencode")
	if err != nil {
		t.Fatalf("LoadMeta failed: %v", err)
	}
	verification := meta.ArtifactVerification
	if verification == nil {
		t.Fatal("the install must record the integrity evidence")
	}
	t.Logf("%s/%s: status=%s published=%s downloaded=%s artifact=%s",
		runtime.GOOS, runtime.GOARCH, verification.Status, verification.Expected, verification.Actual, verification.Artifact)
	if !verification.Verified || verification.Status != agentcfg.ArtifactVerified {
		t.Fatalf("the published digest must match the downloaded artifact, got %+v", verification)
	}
}
