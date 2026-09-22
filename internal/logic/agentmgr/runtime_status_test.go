package agentmgr

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
)

func TestBuildRuntimeReportUsesBoundedProbeStateForOnDemandAgent(t *testing.T) {
	active := true
	input := inspectInput{
		AgentID: "codex", Installed: true,
		Config: AgentConfig{Command: "codex-acp", Kind: "acp", Transport: "stdio", Active: &active},
		State:  RuntimeState{AgentID: "codex", Status: "initialize_failed", Error: "provider process exited with code 1"},
	}

	report := buildRuntimeReport(input, nil)
	if report.Status != "initialize_failed" {
		t.Fatalf("runtime status = %q", report.Status)
	}
	if len(report.Warnings) != 1 || report.Warnings[0] != input.State.Error {
		t.Fatalf("runtime warnings = %+v", report.Warnings)
	}
}

func TestBuildRuntimeReportDoesNotClaimReadyBeforeProbe(t *testing.T) {
	report := buildRuntimeReport(inspectInput{
		AgentID: "codex", Installed: true,
		Config: AgentConfig{Command: "codex-acp", Kind: "acp", Transport: "stdio"},
	}, nil)

	if report.Status != "not_probed" {
		t.Fatalf("runtime status = %q", report.Status)
	}
}

// TestBuildRuntimeReportsExposeArtifactVerification keeps the integrity outcome
// observable to consumers of `matrix doctor`: the answer must reach the runtime
// report, and a missing record must stay absent instead of becoming a false
// "verified".
func TestBuildRuntimeReportsExposeArtifactVerification(t *testing.T) {
	store := memstore.New()
	verified := &agentcfg.ArtifactVerification{
		Verified: true, Status: agentcfg.ArtifactVerified,
		Platform: "linux-x86_64", Expected: "aa", Actual: "aa",
	}
	for agentID, verification := range map[string]*agentcfg.ArtifactVerification{
		"opencode": verified,
		"seeded":   nil,
	} {
		if err := agentcfg.SaveEntry(store, agentID, agentcfg.Entry{
			Config: agentcfg.Config{Command: "matrix-test-command", Kind: "acp", Transport: "stdio"},
		}); err != nil {
			t.Fatalf("SaveEntry failed: %v", err)
		}
		if err := agentcfg.SaveMeta(store, agentID, agentcfg.Meta{ID: agentID, ArtifactVerification: verification}); err != nil {
			t.Fatalf("SaveMeta failed: %v", err)
		}
	}

	registry, err := NewRegistry(nil, store)
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}
	reports, _, err := BuildRuntimeReports(store, registry, execprovider.NewProvider(), func(string) bool { return false })
	if err != nil {
		t.Fatalf("BuildRuntimeReports failed: %v", err)
	}

	byID := make(map[string]AgentRuntimeReport, len(reports))
	for _, report := range reports {
		byID[report.AgentID] = report
	}
	recorded, ok := byID["opencode"]
	if !ok {
		t.Fatalf("the report must cover every registered agent, got %+v", reports)
	}
	if recorded.ArtifactVerification == nil || !recorded.ArtifactVerification.Verified {
		t.Fatalf("the digest outcome must reach the runtime report, got %+v", recorded.ArtifactVerification)
	}
	if seeded := byID["seeded"]; seeded.ArtifactVerification != nil {
		t.Fatalf("an agent with no recorded evidence must stay absent, got %+v", seeded.ArtifactVerification)
	}
}
