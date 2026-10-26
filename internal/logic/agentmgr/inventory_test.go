package agentmgr

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/middleware"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
	networkprovider "github.com/Josepavese/matrix/internal/providers/network"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// inventorySentinel is a credential that exists only in the test: if it appears
// in the inventory, the inventory printed a value instead of a name.
const inventorySentinel = "SENTINEL-INVENTORY-7b3f"

// newReadSideInstaller wires the production providers over an isolated vault, a
// temporary PAL home and its agents directory. The home is what the store-backed
// paths need; the registry URL may be empty when the test never fetches.
func newReadSideInstaller(t *testing.T, registryURL string) (*Installer, middleware.Storage, string, string) {
	t.Helper()

	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)

	storage := memstore.New()
	home := t.TempDir()
	baseDir := filepath.Join(home, "agents")
	if err := osfs.NewFSProvider().MkdirAll(baseDir, 0o700); err != nil {
		t.Fatalf("cannot create the test agents dir: %v", err)
	}
	registry := NewRegistryClient(networkprovider.NewProvider(), registryURL)
	registry.goos = "linux"
	registry.arch = "amd64"
	installer, err := NewInstaller(InstallerConfig{
		Net:      networkprovider.NewProvider(),
		Archive:  osfs.NewArchiveProvider(),
		Storage:  storage,
		FS:       osfs.NewFSProvider(),
		Process:  execprovider.NewProvider(),
		Registry: registry,
		BaseDir:  baseDir,
		Home:     home,
	})
	if err != nil {
		t.Fatalf("NewInstaller failed: %v", err)
	}
	return installer, storage, home, baseDir
}

func saveAgentEntry(t *testing.T, storage middleware.Storage, id string, cfg agentcfg.Config) {
	t.Helper()
	if err := agentcfg.SaveEntry(storage, id, agentcfg.Entry{Config: cfg}); err != nil {
		t.Fatalf("SaveEntry(%s): %v", id, err)
	}
}

func saveAgentMeta(t *testing.T, storage middleware.Storage, id string, meta agentcfg.Meta) {
	t.Helper()
	if err := agentcfg.SaveMeta(storage, id, meta); err != nil {
		t.Fatalf("SaveMeta(%s): %v", id, err)
	}
}

func inventoryJSON(t *testing.T, inventory Inventory) string {
	t.Helper()
	data, err := json.Marshal(inventory)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	return string(data)
}

func TestInventoryIsSortedDeterministicAndSecretFree(t *testing.T) {
	installer, storage, _, _ := newReadSideInstaller(t, "")
	recordedAt := time.Date(2020, 2, 2, 2, 2, 2, 0, time.UTC)
	saveAgentEntry(t, storage, "zeta", agentcfg.Config{
		Command: "agent-zeta", Args: []string{"--serve"}, Kind: "acp", Transport: "stdio",
		Env: []string{"MATRIX_TOKEN=" + inventorySentinel, "PLAIN=1", "MATRIX_TOKEN=changed"},
	})
	saveAgentMeta(t, storage, "zeta", agentcfg.Meta{
		ID: "zeta", Name: "Zeta", Version: "2.0.0", DistTypes: []string{"binary"},
		ArtifactVerification: &agentcfg.ArtifactVerification{Verified: true, Status: agentcfg.ArtifactVerified, Platform: testPlatform, VerifiedAt: recordedAt},
	})
	saveAgentEntry(t, storage, "alpha", agentcfg.Config{Command: "npx", Args: []string{"-y", "pkg"}, Kind: "acp", Transport: "stdio"})
	saveAgentMeta(t, storage, "alpha", agentcfg.Meta{ID: "alpha", Name: "Alpha", Version: "1.0.0", DistTypes: []string{"npx"}})

	first, err := installer.Inventory()
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	second, err := installer.Inventory()
	if err != nil {
		t.Fatalf("Inventory again: %v", err)
	}
	if first.SchemaVersion != InventorySchemaVersion {
		t.Fatalf("schema version = %d, want %d", first.SchemaVersion, InventorySchemaVersion)
	}
	if len(first.Agents) != 2 || first.Agents[0].ID != "alpha" || first.Agents[1].ID != "zeta" {
		t.Fatalf("agents are not sorted by id: %+v", first.Agents)
	}
	if firstJSON, secondJSON := inventoryJSON(t, first), inventoryJSON(t, second); firstJSON != secondJSON {
		t.Fatalf("inventory is not deterministic:\n%s\n%s", firstJSON, secondJSON)
	}

	output := inventoryJSON(t, first)
	if strings.Contains(output, inventorySentinel) {
		t.Fatalf("inventory printed an environment value:\n%s", output)
	}
	if strings.Contains(output, recordedAt.Format(time.RFC3339)) {
		t.Fatalf("inventory printed an install timestamp:\n%s", output)
	}
	zeta := first.Agents[1]
	if strings.Join(zeta.EnvKeys, ",") != "MATRIX_TOKEN,PLAIN" {
		t.Fatalf("env keys = %v, want the sorted unique names", zeta.EnvKeys)
	}
	if zeta.Launcher != "agent-zeta" || len(zeta.Args) != 1 || zeta.Args[0] != "--serve" {
		t.Fatalf("launcher/args = %q %v", zeta.Launcher, zeta.Args)
	}
	if zeta.Artifact == nil || !zeta.Artifact.Verified {
		t.Fatalf("artifact evidence missing from inventory: %+v", zeta.Artifact)
	}
	if !zeta.Regenerable || zeta.RestoreCommand != "matrix install zeta" {
		t.Fatalf("registry agent inventory = %+v", zeta)
	}
	if alpha := first.Agents[0]; alpha.Distribution != "npx" || !alpha.Regenerable {
		t.Fatalf("npx agent inventory = %+v", alpha)
	}
}

func TestInventoryRegenerableAndRestoreCommandFollowTheSoftwareSource(t *testing.T) {
	installer, storage, _, _ := newReadSideInstaller(t, "")
	saveAgentEntry(t, storage, "seeded", agentcfg.Config{Command: "seeded-binary", Kind: "acp", Transport: "stdio"})
	saveAgentEntry(t, storage, "remote", agentcfg.Config{Kind: "a2a", Transport: "JSONRPC", Address: "https://example.invalid/a2a"})
	saveAgentEntry(t, storage, "stored", agentcfg.Config{Command: "stored-binary", Kind: "acp", Transport: "stdio"})
	saveAgentMeta(t, storage, "stored", agentcfg.Meta{
		ID: "stored", Version: "3.1.4",
		Reference: &agentcfg.ArtifactReference{
			Store: "/var/lib/matrix-agent-store", Digest: strings.Repeat("a", sha256HexLength),
			TreeManifestSHA256: strings.Repeat("b", sha256HexLength), Launcher: "bin/stored",
			Version: "3.1.4", Platform: testPlatform,
		},
	})

	inventory, err := installer.Inventory()
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	byID := map[string]InventoryAgent{}
	for _, agent := range inventory.Agents {
		byID[agent.ID] = agent
	}
	if seeded := byID["seeded"]; seeded.Regenerable || seeded.RestoreCommand != "" {
		t.Fatalf("seeded agent is not regenerable: %+v", seeded)
	}
	if remote := byID["remote"]; remote.Regenerable || remote.RestoreCommand != "" {
		t.Fatalf("remote endpoint is not regenerable: %+v", remote)
	}
	stored := byID["stored"]
	if !stored.Regenerable || stored.Store == nil {
		t.Fatalf("store-backed agent inventory = %+v", stored)
	}
	if stored.Store.Digest != strings.Repeat("a", sha256HexLength) || stored.Store.Platform != testPlatform {
		t.Fatalf("store reference = %+v", stored.Store)
	}
	wantCommand := "matrix install stored --store /var/lib/matrix-agent-store --digest " + strings.Repeat("a", sha256HexLength)
	if stored.RestoreCommand != wantCommand {
		t.Fatalf("restore command = %q, want %q", stored.RestoreCommand, wantCommand)
	}
	if byID["stored"].Name != "stored" {
		t.Fatalf("name = %q, want the id when metadata has no name", byID["stored"].Name)
	}
}

func TestInventoryUnknownArtifactEvidenceIsNotAVerification(t *testing.T) {
	installer, storage, _, _ := newReadSideInstaller(t, "")
	saveAgentEntry(t, storage, "legacy", agentcfg.Config{Command: "legacy-binary"})
	saveAgentMeta(t, storage, "legacy", agentcfg.Meta{ID: "legacy", Version: "0.9.0", DistTypes: []string{"binary"}})

	inventory, err := installer.Inventory()
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if len(inventory.Agents) != 1 || inventory.Agents[0].Artifact != nil {
		t.Fatalf("missing install evidence must stay absent, got %+v", inventory.Agents)
	}
	if !inventory.Agents[0].Regenerable {
		t.Fatal("an agent with a binary distribution is regenerable even without recorded evidence")
	}
}
