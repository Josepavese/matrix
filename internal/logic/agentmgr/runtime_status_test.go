package agentmgr

import "testing"

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
