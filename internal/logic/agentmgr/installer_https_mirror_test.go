package agentmgr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
	networkprovider "github.com/Josepavese/matrix/internal/providers/network"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

func TestInstallFromHTTPSMirrorWithoutGitHub(t *testing.T) {
	artifact := testArtifactFiles(t, map[string]string{"opencode": "#!/bin/sh\nexit 0\n"})
	digest := sha256Hex(artifact)
	var indexHits, artifactHits atomic.Int32
	var mirror *httptest.Server
	mirror = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry.json":
			indexHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"version": "mirror-fixture", "agents": []interface{}{map[string]interface{}{
					"id": "opencode", "name": "OpenCode", "version": "9.9.9",
					"distribution": map[string]interface{}{"binary": map[string]interface{}{testPlatform: map[string]interface{}{
						"archive": mirror.URL + "/artifact.tar.gz", "cmd": "./opencode", "sha256": digest,
					}}},
				}},
			})
		case "/artifact.tar.gz":
			artifactHits.Add(1)
			_, _ = w.Write(artifact)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mirror.Close()
	net := networkprovider.NewProviderWithTransport(mirror.Client().Transport)
	registry := NewRegistryClient(net, mirror.URL+"/registry.json")
	registry.goos, registry.arch = "linux", "amd64"
	store := memstore.New()
	baseDir := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	installer, err := NewInstaller(InstallerConfig{Net: net, Archive: osfs.NewArchiveProvider(), Storage: store,
		FS: osfs.NewFSProvider(), Process: execprovider.NewProvider(), Registry: registry, BaseDir: baseDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("install from HTTPS mirror: %v", err)
	}
	meta, err := agentcfg.LoadMeta(store, "opencode")
	if err != nil || meta.ArtifactVerification == nil || !meta.ArtifactVerification.Verified ||
		meta.ArtifactVerification.Expected != digest || meta.ArtifactVerification.Actual != digest {
		t.Fatalf("missing mirror digest proof: %+v err=%v", meta.ArtifactVerification, err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "opencode", "opencode")); err != nil {
		t.Fatalf("verified mirror artifact not installed: %v", err)
	}
	if indexHits.Load() != 1 || artifactHits.Load() != 1 {
		t.Fatalf("install did not use only the HTTPS mirror: index=%d artifact=%d", indexHits.Load(), artifactHits.Load())
	}
}
