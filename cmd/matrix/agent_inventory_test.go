package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/Josepavese/matrix/internal/logic/agentstore"
	"github.com/spf13/cobra"
)

const cliInventorySentinel = "SENTINEL-CLI-INVENTORY-41d2"

func sampleInventory() agentmgr.Inventory {
	return agentmgr.Inventory{SchemaVersion: agentmgr.InventorySchemaVersion, Agents: []agentmgr.InventoryAgent{
		{
			ID: "alpha", Version: "1.0.0", Distribution: "npx", Launcher: "npx",
			EnvKeys: []string{"MATRIX_TOKEN"}, Regenerable: true, RestoreCommand: "matrix install alpha",
		},
		{
			ID: "beta", Version: "", Distribution: "",
			Artifact:    &agentmgr.InventoryArtifact{Verified: true, Status: agentcfg.ArtifactVerified},
			Store:       &agentmgr.StoreReference{Store: "/srv/store", Digest: strings.Repeat("a", 64), Platform: "linux-x86_64"},
			Regenerable: true, RestoreCommand: "matrix install beta --store /srv/store --digest " + strings.Repeat("a", 64),
		},
		{ID: "seeded", Regenerable: false},
	}}
}

func TestInventoryLinesAreTabularAndNameOnlySafeFacts(t *testing.T) {
	lines := inventoryLines(sampleInventory())
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want a header and one line per agent", len(lines))
	}
	if !strings.HasPrefix(lines[0], "ID\tVERSION\t") {
		t.Fatalf("header = %q", lines[0])
	}
	alpha := strings.Split(lines[1], "\t")
	if alpha[0] != "alpha" || alpha[3] != "not recorded" || alpha[4] != "yes" || alpha[5] != "-" {
		t.Fatalf("unrecorded artifact row = %v", alpha)
	}
	beta := strings.Split(lines[2], "\t")
	if beta[1] != "-" || beta[2] != "-" || beta[3] != string(agentcfg.ArtifactVerified) {
		t.Fatalf("store-backed row = %v", beta)
	}
	if beta[5] != strings.Repeat("a", 64) {
		t.Fatalf("store digest column = %q", beta[5])
	}
	seeded := strings.Split(lines[3], "\t")
	if seeded[4] != "no" {
		t.Fatalf("seeded row = %v", seeded)
	}
	output := strings.Join(lines, "\n")
	if strings.Contains(output, cliInventorySentinel) {
		t.Fatalf("the table printed an environment value:\n%s", output)
	}
}

func TestEmitJSONWritesOneParseableDocument(t *testing.T) {
	var buffer bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&buffer)
	emitJSON(command, sampleInventory())

	var decoded agentmgr.Inventory
	if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil {
		t.Fatalf("emitted JSON does not parse: %v\n%s", err, buffer.String())
	}
	if decoded.SchemaVersion != agentmgr.InventorySchemaVersion || len(decoded.Agents) != 3 {
		t.Fatalf("emitted inventory = %+v", decoded)
	}
}

func TestVerifyLinesAndStrictVerdict(t *testing.T) {
	results := []agentmgr.VerifyResult{
		{ID: "alpha", Method: agentmgr.VerifyMethodStoreTree, Verified: true, Detail: "already proven"},
		{ID: "beta", Method: agentmgr.VerifyMethodRecordedEvidence, Verified: false, Detail: "not proven"},
	}
	lines := verifyLines(results)
	if len(lines) != 2 || !strings.Contains(lines[0], "OK") || !strings.Contains(lines[1], "FAILED") {
		t.Fatalf("verify lines = %v", lines)
	}
	if !strings.Contains(lines[1], agentmgr.VerifyMethodRecordedEvidence) {
		t.Fatalf("verify line hides the method: %q", lines[1])
	}
	if !verifyAnyFailed(results) {
		t.Fatal("a failed result must fail the strict verdict")
	}
	if verifyAnyFailed(results[:1]) {
		t.Fatal("a fully verified batch must pass the strict verdict")
	}
}

func TestPruneLinesKeepTheRefusal(t *testing.T) {
	ids := []string{"stored", "seeded"}
	results := []agentmgr.PruneResult{{ID: "stored", Bytes: 42, Removed: false, Detail: "would remove"}}
	failures := map[string]string{"seeded": "no verified store reference"}
	lines := pruneLines(ids, results, failures)
	if len(lines) != 2 {
		t.Fatalf("prune lines = %v", lines)
	}
	if !strings.Contains(lines[0], "42") || !strings.Contains(lines[0], "removed=false") {
		t.Fatalf("prune result line = %q", lines[0])
	}
	if !strings.Contains(lines[1], "refused") || !strings.Contains(lines[1], "no verified store reference") {
		t.Fatalf("prune refusal line = %q", lines[1])
	}
}

func TestStoreRootResolutionUsesTheDeploymentContract(t *testing.T) {
	if _, err := resolveStoreRoot(""); err == nil || !strings.Contains(err.Error(), agentstore.EnvName) {
		t.Fatalf("a missing store path must name %s: %v", agentstore.EnvName, err)
	}
	t.Setenv(agentstore.EnvName, "/srv/agent-store")
	root, err := resolveStoreRoot("")
	if err != nil || root != "/srv/agent-store" {
		t.Fatalf("env store root = %q err=%v", root, err)
	}
	root, err = resolveStoreRoot("/explicit")
	if err != nil || root != "/explicit" {
		t.Fatalf("flag store root wins: %q err=%v", root, err)
	}
}

func TestGCEntriesCarryTheReclaimableFacts(t *testing.T) {
	entries := gcEntries([]agentstore.Pending{
		{Digest: strings.Repeat("b", 64), AgentID: "alpha", Version: "1.0.0", Platform: "linux-x86_64", Bytes: 2048},
	})
	if len(entries) != 1 || entries[0].Bytes != 2048 || entries[0].Digest != strings.Repeat("b", 64) {
		t.Fatalf("gc entries = %+v", entries)
	}
}

func TestNewAgentSubcommandsAreRegisteredWithTheirFlags(t *testing.T) {
	registered := map[string]*cobra.Command{}
	for _, command := range agentCmd.Commands() {
		registered[command.Name()] = command
	}
	for _, name := range []string{"inventory", "verify", "restore", "prune", "store"} {
		if registered[name] == nil {
			t.Fatalf("`matrix agent %s` is not registered (registered: %v)", name, mapKeys(registered))
		}
	}
	storeCommands := map[string]*cobra.Command{}
	for _, command := range registered["store"].Commands() {
		storeCommands[command.Name()] = command
	}
	for _, name := range []string{"publish", "list", "verify", "gc"} {
		if storeCommands[name] == nil {
			t.Fatalf("`matrix agent store %s` is not registered", name)
		}
	}
	requiredFlags := map[string][]string{
		"inventory": {"json"},
		"verify":    {"json", "strict"},
		"restore":   {"inventory", "all", "store", "allow-version-drift", "replace", "dry-run", "json"},
		"prune":     {"apply", "json"},
	}
	for name, flags := range requiredFlags {
		for _, flag := range flags {
			if registered[name].Flags().Lookup(flag) == nil {
				t.Fatalf("`matrix agent %s` is missing --%s", name, flag)
			}
		}
	}
	for _, name := range []string{"publish", "list", "verify", "gc"} {
		if storeCommands[name].Flags().Lookup("store") == nil || storeCommands[name].Flags().Lookup("json") == nil {
			t.Fatalf("`matrix agent store %s` is missing --store/--json", name)
		}
	}
	if storeCommands["gc"].Flags().Lookup("home") == nil || storeCommands["gc"].Flags().Lookup("apply") == nil {
		t.Fatal("`matrix agent store gc` is missing --home/--apply")
	}
}

func mapKeys(commands map[string]*cobra.Command) []string {
	keys := make([]string, 0, len(commands))
	for name := range commands {
		keys = append(keys, name)
	}
	return keys
}

// TestVerifyLinesKeepTheRemedyBesideTheVerdict locks the human surface: a result
// that carries a remedy shows it in the same line, because an operator who sees
// FAILED must also see what would fix it. The line keeps its four columns.
func TestVerifyLinesKeepTheRemedyBesideTheVerdict(t *testing.T) {
	results := []agentmgr.VerifyResult{
		{ID: "alpha", Method: agentmgr.VerifyMethodStoreWritable, Detail: "measured but not durable", Remedy: "publish from a different account"},
		{ID: "beta", Method: agentmgr.VerifyMethodStoreTree, Verified: true, Detail: "already proven"},
	}
	lines := verifyLines(results)
	if len(lines) != 2 {
		t.Fatalf("verify lines = %d, want one per result", len(lines))
	}
	if !strings.Contains(lines[0], "remedy: publish from a different account") {
		t.Fatalf("the failing line hides the remedy: %q", lines[0])
	}
	if strings.Contains(lines[1], "remedy:") {
		t.Fatalf("a result without a remedy must not print one: %q", lines[1])
	}
	if fields := strings.Split(lines[0], "\t"); len(fields) != 4 {
		t.Fatalf("the line changed its column count: %d fields in %q", len(fields), lines[0])
	}
}
