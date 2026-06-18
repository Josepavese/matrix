package agentmgr

import (
	"path/filepath"
	"testing"
)

func TestAgentDirValidatesAgentID(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "agents")

	got, err := agentDir(baseDir, "opencode")
	if err != nil {
		t.Fatalf("valid agent id rejected: %v", err)
	}
	if got != filepath.Join(baseDir, "opencode") {
		t.Fatalf("agent dir = %q, want %q", got, filepath.Join(baseDir, "opencode"))
	}

	for _, agentID := range []string{"", ".", "..", "../data", "nested/agent", `nested\agent`, filepath.Join(baseDir, "abs")} {
		t.Run(agentID, func(t *testing.T) {
			if _, err := agentDir(baseDir, agentID); err == nil {
				t.Fatalf("expected invalid agent id %q to be rejected", agentID)
			}
		})
	}
}

func TestAgentTempArchiveValidatesPathTokens(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "tmp")

	got, err := agentTempArchive(tempDir, "opencode", "1.2.3", "https://example.test/agent.tar.gz")
	if err != nil {
		t.Fatalf("valid temp archive rejected: %v", err)
	}
	want := filepath.Join(tempDir, "matrix-agent-opencode-1.2.3.tar.gz")
	if got != want {
		t.Fatalf("temp archive = %q, want %q", got, want)
	}

	for _, tc := range []struct {
		name    string
		agentID string
		version string
	}{
		{name: "traversal version", agentID: "opencode", version: "../../../../tmp/victim"},
		{name: "absolute version", agentID: "opencode", version: "/tmp/victim"},
		{name: "nested version", agentID: "opencode", version: "nested/version"},
		{name: "traversal agent", agentID: "../opencode", version: "1.2.3"},
		{name: "empty version", agentID: "opencode", version: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := agentTempArchive(tempDir, tc.agentID, tc.version, "https://example.test/agent.tar.gz"); err == nil {
				t.Fatalf("expected invalid temp archive token to be rejected")
			}
		})
	}
}
