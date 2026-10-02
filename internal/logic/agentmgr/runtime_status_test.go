package agentmgr

import (
	"context"
	"strings"
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

// TestBuildRuntimeReportSaysServedOnDemandWithoutClaimingItWasObserved keeps the
// guard that a status is never a claim about an observation nobody made. An
// active, installed ACP agent over stdio is started by the run that needs it, so
// the status says so — and says nothing more: the warning states that nothing has
// been seen running, and the report carries no pid, address or timestamp, which
// is what an observation would have left behind.
//
// The word is ready_on_demand and not ready for exactly that reason: "I start it
// when you ask" is not "I watched it run", and an operator reading this status
// must not feel authorised to believe the second. The other half of the guard
// stays where an apply really is missing: a transport the runtime does not start
// per run keeps its pending_apply and its remedy.
func TestBuildRuntimeReportSaysServedOnDemandWithoutClaimingItWasObserved(t *testing.T) {
	report := buildRuntimeReport(inspectInput{
		AgentID: "codex", Installed: true,
		Config: AgentConfig{Command: "codex-acp", Kind: "acp", Transport: "stdio"},
	}, nil)

	if report.Status != "ready_on_demand" {
		t.Fatalf("runtime status = %q, want ready_on_demand for an agent the run path starts per run", report.Status)
	}
	if report.Status == "running" {
		t.Fatal("an agent nobody has observed was reported as running")
	}
	want := "served on demand: the runtime starts this agent when a run arrives; nothing has been observed running yet"
	if len(report.Warnings) != 1 || report.Warnings[0] != want {
		t.Fatalf("warning = %q, want %q", report.Warnings, want)
	}
	if report.PID != 0 || report.Address != "" || !report.UpdatedAt.IsZero() {
		t.Fatalf("a status with no observation reported evidence of one: %+v", report)
	}

	// A remote agent is not started by the run path, so it still waits for an
	// apply and still gets the shared remedy: the fix removed a false remedy for
	// one transport, it did not delete the vocabulary for the others.
	remote := buildRuntimeReport(inspectInput{
		AgentID: "remote", Installed: true,
		Config: AgentConfig{Command: "https://peer.example", Kind: "a2a"},
	}, nil)
	if remote.Status != "pending_apply" {
		t.Fatalf("remote status = %q, want pending_apply: nothing about a remote peer is served per run", remote.Status)
	}
	wantRemedy := "registration recorded, runtime not yet applied: " + middleware.RegistrationRemedyThen("re-run this command")
	if len(remote.Warnings) != 1 || remote.Warnings[0] != wantRemedy {
		t.Fatalf("remote warning = %q, want the shared remedy %q", remote.Warnings, wantRemedy)
	}
}

// TestBuildRuntimeReportForOneAgentReportsTheRegisteredState is the acceptance
// evidence for the single-agent report a show command consumes: an agent
// registered in the SSOT and not yet observed by the runtime is reported as
// served on demand — not as an error, not as a run that was watched — and the
// report stays about that agent.
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
	if report.Status != "ready_on_demand" {
		t.Fatalf("runtime status = %q, want ready_on_demand for a registered agent the runtime has not observed", report.Status)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "nothing has been observed running yet") {
		t.Fatalf("the status of an unobserved agent does not say so: %q", report.Warnings)
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
