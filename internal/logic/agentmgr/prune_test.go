package agentmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentinstall"
	"github.com/Josepavese/matrix/internal/logic/agentstore"
	"github.com/Josepavese/matrix/internal/middleware"
)

// savePrunableAgent records an agent whose software lives in a verified store
// generation and whose old per-home copy is still on disk, which is the state
// prune is allowed to reclaim.
func savePrunableAgent(t *testing.T, storage middleware.Storage, home, baseDir string, fixture storeFixture) (string, int64) {
	t.Helper()
	agentID := fixture.generation.AgentID
	saveAgentEntry(t, storage, agentID, agentcfg.Config{
		Command: fixture.launcherPath(), Args: fixture.generation.Args, Kind: "acp", Transport: "stdio",
	})
	saveAgentMeta(t, storage, agentID, agentcfg.Meta{
		ID: agentID, Version: fixture.generation.Version, DistTypes: []string{"binary"},
		Reference: fixture.reference(),
	})
	if err := agentstore.WriteReference(home, agentID, agentstore.Reference{
		Store: fixture.root, Digest: fixture.digest, TreeManifestSHA256: fixture.generation.TreeManifestSHA256,
		Launcher: fixture.generation.Launcher, Version: fixture.generation.Version, Platform: fixture.generation.Platform,
	}); err != nil {
		t.Fatalf("WriteReference: %v", err)
	}
	return writeHomeCopy(t, baseDir, agentID, map[string]string{
		"agent-bin":  "#!/bin/sh\nexit 0\n",
		"share/data": "0123456789",
	})
}

func writeHomeCopy(t *testing.T, baseDir, agentID string, files map[string]string) (string, int64) {
	t.Helper()
	dir, err := agentinstall.AgentDir(baseDir, agentID)
	if err != nil {
		t.Fatalf("AgentDir: %v", err)
	}
	var total int64
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		total += int64(len(content))
	}
	return dir, total
}

func TestPruneCopyRefusesAnAgentWithoutAStoreReference(t *testing.T) {
	installer, storage, _, _ := newReadSideInstaller(t, "")
	saveAgentEntry(t, storage, "local", agentcfg.Config{Command: "/somewhere/local"})

	_, err := installer.PruneCopy("local", false)
	if err == nil || !strings.Contains(err.Error(), "no verified store reference") {
		t.Fatalf("prune of an agent with no store reference returned %v", err)
	}
}

func TestPruneCopyRefusesWhenTheStoreGenerationNoLongerVerifies(t *testing.T) {
	installer, storage, home, baseDir := newReadSideInstaller(t, "")
	fixture := publishStoreFixture(t, "stored", "2.5.0")
	agentDir, _ := savePrunableAgent(t, storage, home, baseDir, fixture)

	makeStoreTreeWritable(fixture.blobDir())
	if err := os.Chmod(fixture.launcherPath(), 0o644); err != nil {
		t.Fatalf("unseal launcher: %v", err)
	}
	if err := os.WriteFile(fixture.launcherPath(), []byte("#!/bin/sh\nexit 3\n"), 0o644); err != nil {
		t.Fatalf("tamper store: %v", err)
	}
	sealStoreLayout(t, fixture.root)

	result, err := installer.PruneCopy("stored", true)
	if err == nil || !strings.Contains(err.Error(), "does not verify") {
		t.Fatalf("prune of a tampered store returned result=%+v err=%v", result, err)
	}
	if _, statErr := os.Stat(filepath.Join(agentDir, "agent-bin")); statErr != nil {
		t.Fatalf("the per-home copy was removed even though the store did not verify: %v", statErr)
	}
	references, refErr := agentstore.ReadReferences(home)
	if refErr != nil || references.Agents["stored"].Digest != fixture.digest {
		t.Fatalf("the store reference was removed on a refused prune: %+v err=%v", references.Agents, refErr)
	}
}

func TestPruneCopyDryRunReportsBytesWithoutChangingAnything(t *testing.T) {
	installer, storage, home, baseDir := newReadSideInstaller(t, "")
	fixture := publishStoreFixture(t, "stored", "2.5.0")
	agentDir, wantBytes := savePrunableAgent(t, storage, home, baseDir, fixture)
	before, err := agentcfg.LoadEntry(storage, "stored")
	if err != nil {
		t.Fatalf("LoadEntry: %v", err)
	}

	result, err := installer.PruneCopy("stored", false)
	if err != nil {
		t.Fatalf("PruneCopy dry run: %v", err)
	}
	if result.Bytes != wantBytes || result.Removed {
		t.Fatalf("dry-run result = %+v, want %d bytes and no removal", result, wantBytes)
	}
	if result.AgentDir != agentDir {
		t.Fatalf("agent dir = %q, want %q", result.AgentDir, agentDir)
	}
	if _, statErr := os.Stat(filepath.Join(agentDir, "agent-bin")); statErr != nil {
		t.Fatalf("a dry run removed the per-home copy: %v", statErr)
	}
	after, err := agentcfg.LoadEntry(storage, "stored")
	if err != nil || after.Config.Command != before.Config.Command {
		t.Fatalf("a dry run changed the vault record: %+v err=%v", after, err)
	}
	references, refErr := agentstore.ReadReferences(home)
	if refErr != nil || references.Agents["stored"].Digest != fixture.digest {
		t.Fatalf("a dry run removed the store reference: %+v err=%v", references.Agents, refErr)
	}
}

func TestPruneCopyApplyRemovesOnlyTheSoftwareCopy(t *testing.T) {
	installer, storage, home, baseDir := newReadSideInstaller(t, "")
	fixture := publishStoreFixture(t, "stored", "2.5.0")
	agentDir, wantBytes := savePrunableAgent(t, storage, home, baseDir, fixture)
	before, err := agentcfg.LoadEntry(storage, "stored")
	if err != nil {
		t.Fatalf("LoadEntry: %v", err)
	}

	result, err := installer.PruneCopy("stored", true)
	if err != nil {
		t.Fatalf("PruneCopy apply: %v", err)
	}
	if !result.Removed || result.Bytes != wantBytes {
		t.Fatalf("apply result = %+v, want %d bytes removed", result, wantBytes)
	}
	if _, statErr := os.Stat(agentDir); !os.IsNotExist(statErr) {
		t.Fatalf("the per-home copy still exists after apply: %v", statErr)
	}
	after, err := agentcfg.LoadEntry(storage, "stored")
	if err != nil || after.Config.Command != before.Config.Command {
		t.Fatalf("prune touched the vault record: %+v err=%v", after, err)
	}
	references, err := agentstore.ReadReferences(home)
	if err != nil {
		t.Fatalf("ReadReferences: %v", err)
	}
	if references.Agents["stored"].Digest != fixture.digest {
		t.Fatalf("the store reference must survive the reclamation, since the agent is still installed from it: %+v", references.Agents)
	}
}

func TestPruneCopyWithNothingToReclaimKeepsTheReference(t *testing.T) {
	installer, storage, home, _ := newReadSideInstaller(t, "")
	fixture := publishStoreFixture(t, "stored", "2.5.0")
	saveAgentEntry(t, storage, "stored", agentcfg.Config{Command: fixture.launcherPath()})
	saveAgentMeta(t, storage, "stored", agentcfg.Meta{ID: "stored", Reference: fixture.reference()})
	if err := agentstore.WriteReference(home, "stored", agentstore.Reference{Store: fixture.root, Digest: fixture.digest, Platform: fixture.generation.Platform}); err != nil {
		t.Fatalf("WriteReference: %v", err)
	}
	result, err := installer.PruneCopy("stored", true)
	if err != nil {
		t.Fatalf("PruneCopy: %v", err)
	}
	if result.Bytes != 0 || result.Removed || !strings.Contains(result.Detail, "nothing to reclaim") {
		t.Fatalf("no-copy result = %+v", result)
	}
	references, err := agentstore.ReadReferences(home)
	if err != nil || references.Agents["stored"].Digest != fixture.digest {
		t.Fatalf("a no-op prune removed the store reference: %+v err=%v", references.Agents, err)
	}
}
