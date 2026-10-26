package main

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentmgr"
)

func validInventoryDocument() string {
	return `{
  "schema_version": 1,
  "agents": [
    {"id": "alpha", "version": "1.0.0", "distribution": "binary", "regenerable": true,
     "store": {"store": "/srv/store", "digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
               "platform": "linux-x86_64"}}
  ],
  "produced_by": "a newer matrix build"
}`
}

func TestParseInventoryDocumentAcceptsSchemaVersionOneAndUnknownFields(t *testing.T) {
	inventory, err := parseInventoryDocument([]byte(validInventoryDocument()))
	if err != nil {
		t.Fatalf("parseInventoryDocument: %v", err)
	}
	if inventory.SchemaVersion != agentmgr.InventorySchemaVersion || len(inventory.Agents) != 1 {
		t.Fatalf("parsed inventory = %+v", inventory)
	}
	agent := inventory.Agents[0]
	if agent.ID != "alpha" || agent.Store == nil || agent.Store.Digest != strings.Repeat("a", 64) {
		t.Fatalf("parsed agent = %+v", agent)
	}
}

func TestParseInventoryDocumentRejectsUnsupportedOrMissingSchema(t *testing.T) {
	cases := map[string]string{
		"missing schema":    `{"agents": []}`,
		"future schema":     `{"schema_version": 2, "agents": []}`,
		"negative schema":   `{"schema_version": -1, "agents": []}`,
		"malformed json":    `{"schema_version": 1, "agents": [}`,
		"trailing document": `{"schema_version": 1, "agents": []}{"schema_version": 1, "agents": []}`,
	}
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseInventoryDocument([]byte(document)); err == nil {
				t.Fatalf("%s must be refused, accepted %q", name, document)
			}
		})
	}
}

func TestRestoreLinesNameActionDetailAndError(t *testing.T) {
	lines := restoreLines([]agentmgr.RestoreResult{
		{ID: "alpha", Action: agentmgr.RestoreActionInstalled, Detail: "installed version 1.0.0"},
		{ID: "beta", Action: agentmgr.RestoreActionDrift, Detail: "recorded 1.0.0, published 2.0.0"},
		{ID: "gamma", Action: agentmgr.RestoreActionFailed, Error: "download failed"},
	})
	if len(lines) != 3 {
		t.Fatalf("restore lines = %v", lines)
	}
	if !strings.HasPrefix(lines[0], "alpha\tinstalled\t") {
		t.Fatalf("installed line = %q", lines[0])
	}
	if !strings.Contains(lines[1], agentmgr.RestoreActionDrift) || !strings.Contains(lines[1], "1.0.0") {
		t.Fatalf("drift line = %q", lines[1])
	}
	if !strings.Contains(lines[2], "error: download failed") {
		t.Fatalf("failed line = %q", lines[2])
	}
	if !restoreAnyError([]agentmgr.RestoreResult{{Action: agentmgr.RestoreActionFailed, Error: "x"}}) {
		t.Fatal("a failed restore must set the command's exit status")
	}
	if restoreAnyError([]agentmgr.RestoreResult{{Action: agentmgr.RestoreActionSkippedPresent}, {Action: agentmgr.RestoreActionDrift}}) {
		t.Fatal("skip and drift results must not set the command's exit status")
	}
}
