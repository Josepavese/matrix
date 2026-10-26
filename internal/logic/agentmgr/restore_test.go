package agentmgr

import (
	"context"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentstore"
)

// inventoryOf builds a one-agent inventory the way the CLI would read it back
// from a saved document.
func inventoryOf(agent InventoryAgent) Inventory {
	return Inventory{SchemaVersion: InventorySchemaVersion, Agents: []InventoryAgent{agent}}
}

func registryAgent(agentID, version string) InventoryAgent {
	return InventoryAgent{ID: agentID, Version: version, Distribution: "binary", Regenerable: true}
}

func restoreByID(results []RestoreResult) map[string]RestoreResult {
	byID := make(map[string]RestoreResult, len(results))
	for _, result := range results {
		byID[result.ID] = result
	}
	return byID
}

func TestRestoreSkipsAnAlreadyRegisteredAgent(t *testing.T) {
	fake := startFakeRegistry(t, publishRealDigest)
	installer, storage, _, _ := newReadSideInstaller(t, fake.registryURL())
	saveAgentEntry(t, storage, "opencode", agentcfg.Config{Command: "/somewhere/opencode"})

	results, err := installer.Restore(context.Background(), inventoryOf(registryAgent("opencode", "9.9.9")), RestoreOptions{})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(results) != 1 || results[0].Action != RestoreActionSkippedPresent {
		t.Fatalf("already-present restore = %+v", results)
	}
	if results[0].Error != "" || !strings.Contains(results[0].Detail, "--replace") {
		t.Fatalf("skip result = %+v", results[0])
	}
}

func TestRestoreRefusesVersionDriftWithoutInstalling(t *testing.T) {
	fake := startFakeRegistry(t, publishRealDigest)
	installer, storage, _, _ := newReadSideInstaller(t, fake.registryURL())

	results, err := installer.Restore(context.Background(), inventoryOf(registryAgent("opencode", "1.0.0")), RestoreOptions{})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	result := results[0]
	if result.Action != RestoreActionDrift || result.Error != "" {
		t.Fatalf("drift result = %+v", result)
	}
	if !strings.Contains(result.Detail, "1.0.0") || !strings.Contains(result.Detail, "9.9.9") {
		t.Fatalf("drift detail must name both versions: %q", result.Detail)
	}
	if ids, err := agentcfg.ListAgentIDs(storage); err != nil || len(ids) != 0 {
		t.Fatalf("a drifted restore must not install: ids=%v err=%v", ids, err)
	}
}

func TestRestoreInstallsTheMatchingRegistryVersion(t *testing.T) {
	fake := startFakeRegistry(t, publishRealDigest)
	installer, storage, _, _ := newReadSideInstaller(t, fake.registryURL())

	results, err := installer.Restore(context.Background(), inventoryOf(registryAgent("opencode", "9.9.9")), RestoreOptions{})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if results[0].Action != RestoreActionInstalled || results[0].Error != "" {
		t.Fatalf("install result = %+v", results[0])
	}
	meta, err := agentcfg.LoadMeta(storage, "opencode")
	if err != nil || meta.Version != "9.9.9" {
		t.Fatalf("restored meta version = %q err=%v", meta.Version, err)
	}
	entry, err := agentcfg.LoadEntry(storage, "opencode")
	if err != nil || entry.Config.Command == "" {
		t.Fatalf("restored entry = %+v err=%v", entry, err)
	}
}

func TestRestoreDryRunReportsWithoutTouchingAnything(t *testing.T) {
	fake := startFakeRegistry(t, publishRealDigest)
	installer, storage, _, _ := newReadSideInstaller(t, fake.registryURL())

	results, err := installer.Restore(context.Background(), inventoryOf(registryAgent("opencode", "9.9.9")), RestoreOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if results[0].Action != RestoreActionWouldInstall || results[0].Error != "" {
		t.Fatalf("dry-run result = %+v", results[0])
	}
	if ids, _ := agentcfg.ListAgentIDs(storage); len(ids) != 0 {
		t.Fatalf("a dry run must not install, ids=%v", ids)
	}
}

func TestRestoreAllowVersionDriftInstallsThePublishedVersion(t *testing.T) {
	fake := startFakeRegistry(t, publishRealDigest)
	installer, storage, _, _ := newReadSideInstaller(t, fake.registryURL())

	results, err := installer.Restore(context.Background(), inventoryOf(registryAgent("opencode", "1.0.0")), RestoreOptions{AllowVersionDrift: true})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if results[0].Action != RestoreActionInstalled || results[0].Error != "" {
		t.Fatalf("drift-allowed result = %+v", results[0])
	}
	meta, err := agentcfg.LoadMeta(storage, "opencode")
	if err != nil || meta.Version != "9.9.9" {
		t.Fatalf("installed version = %q err=%v, want the published one", meta.Version, err)
	}
}

func TestRestoreReplaceReinstallsAnAlreadyRegisteredAgent(t *testing.T) {
	fake := startFakeRegistry(t, publishRealDigest)
	installer, storage, _, _ := newReadSideInstaller(t, fake.registryURL())
	saveAgentEntry(t, storage, "opencode", agentcfg.Config{Command: "/old/path"})

	results, err := installer.Restore(context.Background(), inventoryOf(registryAgent("opencode", "9.9.9")), RestoreOptions{Replace: true, DryRun: true})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if results[0].Action != RestoreActionWouldInstall {
		t.Fatalf("--replace restore = %+v", results[0])
	}
}

func TestRestoreLeavesNonRegenerableAgentsAloneWithoutNetwork(t *testing.T) {
	installer, storage, _, _ := newReadSideInstaller(t, "")
	installer.registry = nil
	saveAgentEntry(t, storage, "seeded", agentcfg.Config{Command: "seed-binary"})

	results, err := installer.Restore(context.Background(), inventoryOf(InventoryAgent{ID: "seeded"}), RestoreOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if results[0].Action != RestoreActionNotApplicable || results[0].Error != "" {
		t.Fatalf("non-regenerable result = %+v", results[0])
	}
}

func TestRestoreStoreBackedAgentInstallsTheVerifiedGeneration(t *testing.T) {
	installer, storage, home, _ := newReadSideInstaller(t, "")
	fixture := publishStoreFixture(t, "stored", "2.5.0")
	agent := InventoryAgent{
		ID: "stored", Version: fixture.generation.Version, Regenerable: true,
		Store: &StoreReference{
			Store: fixture.root, Digest: fixture.digest, TreeManifestSHA256: fixture.generation.TreeManifestSHA256,
			Launcher: fixture.generation.Launcher, Version: fixture.generation.Version, Platform: fixture.generation.Platform,
		},
	}

	results, err := installer.Restore(context.Background(), inventoryOf(agent), RestoreOptions{StoreRoot: fixture.root})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if results[0].Action != RestoreActionInstalled || results[0].Error != "" {
		t.Fatalf("store-backed restore = %+v", results[0])
	}
	references, err := agentstore.ReadReferences(home)
	if err != nil {
		t.Fatalf("ReadReferences: %v", err)
	}
	if ref, ok := references.Agents["stored"]; !ok || ref.Digest != fixture.digest {
		t.Fatalf("store reference not written: %+v", references.Agents)
	}
	meta, err := agentcfg.LoadMeta(storage, "stored")
	if err != nil || meta.Reference == nil || meta.Reference.Digest != fixture.digest {
		t.Fatalf("vault metadata lost the store reference: %+v err=%v", meta, err)
	}
	entry, err := agentcfg.LoadEntry(storage, "stored")
	if err != nil || entry.Config.Command != fixture.launcherPath() {
		t.Fatalf("store-backed launcher = %q err=%v, want %q", entry.Config.Command, err, fixture.launcherPath())
	}
}

func TestRestoreStoreBackedDryRunWritesNoReference(t *testing.T) {
	installer, storage, home, _ := newReadSideInstaller(t, "")
	fixture := publishStoreFixture(t, "stored", "2.5.0")
	agent := InventoryAgent{
		ID: "stored", Version: fixture.generation.Version, Regenerable: true,
		Store: &StoreReference{Store: fixture.root, Digest: fixture.digest, Platform: fixture.generation.Platform},
	}

	results, err := installer.Restore(context.Background(), inventoryOf(agent), RestoreOptions{StoreRoot: fixture.root, DryRun: true})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if results[0].Action != RestoreActionWouldInstall {
		t.Fatalf("store-backed dry run = %+v", results[0])
	}
	references, err := agentstore.ReadReferences(home)
	if err != nil || len(references.Agents) != 0 {
		t.Fatalf("a dry run wrote a reference: %+v err=%v", references.Agents, err)
	}
	if ids, _ := agentcfg.ListAgentIDs(storage); len(ids) != 0 {
		t.Fatalf("a dry run wrote a vault record: %v", ids)
	}
}

func TestRestoreReportsAnUnavailableRegistryAgentAsFailed(t *testing.T) {
	fake := startFakeRegistry(t, publishRealDigest)
	installer, _, _, _ := newReadSideInstaller(t, fake.registryURL())

	results, err := installer.Restore(context.Background(), inventoryOf(registryAgent("not-in-index", "1.0.0")), RestoreOptions{})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if results[0].Action != RestoreActionFailed || results[0].Error == "" {
		t.Fatalf("unavailable agent result = %+v", results[0])
	}
}

func TestRestoreKeepsTheInventoryOrder(t *testing.T) {
	fake := startFakeRegistry(t, publishRealDigest)
	installer, _, _, _ := newReadSideInstaller(t, fake.registryURL())
	inventory := Inventory{SchemaVersion: InventorySchemaVersion, Agents: []InventoryAgent{
		{ID: "seeded", Regenerable: false},
		registryAgent("opencode", "1.0.0"),
	}}

	results, err := installer.Restore(context.Background(), inventory, RestoreOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(results) != 2 || results[0].ID != "seeded" || results[1].ID != "opencode" {
		t.Fatalf("restore reordered the inventory: %+v", results)
	}
	if results[0].Action != RestoreActionNotApplicable || results[1].Action != RestoreActionDrift {
		t.Fatalf("restore outcomes = %+v", results)
	}
}
