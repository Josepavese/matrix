package agentmgr

import (
	"context"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/middleware"
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

// TestBuildRuntimeReportWaitsForTheRuntimeInsteadOfClaimingReady keeps the
// guard that a registered agent is never reported as ready before the runtime
// observed it. The status names what is missing — an apply — rather than the
// probe that has not run, so a freshly registered agent does not read as a
// fault while the daemon is still catching up.
func TestBuildRuntimeReportWaitsForTheRuntimeInsteadOfClaimingReady(t *testing.T) {
	report := buildRuntimeReport(inspectInput{
		AgentID: "codex", Installed: true,
		Config: AgentConfig{Command: "codex-acp", Kind: "acp", Transport: "stdio"},
	}, nil)

	if report.Status != "pending_apply" {
		t.Fatalf("runtime status = %q, want pending_apply", report.Status)
	}
	if len(report.Warnings) == 0 {
		t.Fatal("pending_apply must tell the operator what to do next")
	}
}

// TestBuildRuntimeReportForOneAgentReportsTheRegisteredState is the acceptance
// evidence for the single-agent report a show command consumes: an agent
// registered in the SSOT and not yet observed by the runtime is reported as
// pending_apply, not as an error, and the report stays about that agent.
func TestBuildRuntimeReportForOneAgentReportsTheRegisteredState(t *testing.T) {
	store := memstore.New()
	if err := agentcfg.SaveEntry(store, "mimo", agentcfg.Entry{Config: AgentConfig{
		Command: "/bin/true", Kind: string(middleware.ProtocolKindACP), Transport: "stdio",
	}}); err != nil {
		t.Fatal(err)
	}
	registry, err := NewRegistry(nil, store)
	if err != nil {
		t.Fatal(err)
	}

	report, err := BuildRuntimeReport(RuntimeReportRequest{
		Store:    store,
		Registry: registry,
		Process:  alwaysInstalledProcess{},
		AgentID:  "mimo",
	})
	if err != nil {
		t.Fatalf("BuildRuntimeReport: %v", err)
	}
	if report.AgentID != "mimo" {
		t.Fatalf("report is about %q", report.AgentID)
	}
	if report.Status != "pending_apply" {
		t.Fatalf("runtime status = %q, want pending_apply for a registered agent the runtime has not observed", report.Status)
	}
}

// alwaysInstalledProcess reports every configured command as present, so a
// report test exercises the runtime vocabulary instead of the host's PATH.
type alwaysInstalledProcess struct{}

func (alwaysInstalledProcess) Exec(middleware.CommandSpec) ([]byte, error) { return nil, nil }
func (alwaysInstalledProcess) ExecSeparate(context.Context, middleware.CommandSpec) (*middleware.ExecResult, error) {
	return nil, nil
}
func (alwaysInstalledProcess) Start(middleware.CommandSpec) (middleware.ProcessHandle, error) {
	return nil, nil
}
func (alwaysInstalledProcess) StartPiped(middleware.CommandSpec) (middleware.PipedProcess, error) {
	return nil, nil
}
func (alwaysInstalledProcess) RunPrivileged(middleware.CommandSpec) ([]byte, error) { return nil, nil }
func (alwaysInstalledProcess) HasExecutable(string) bool                            { return true }
func (alwaysInstalledProcess) SpawnPTY() error                                      { return nil }

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
