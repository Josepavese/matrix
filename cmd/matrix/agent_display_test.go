package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
)

// A credential that exists in the test and nowhere else: if it appears in what a
// reporting command prints, the command printed a value.
const displaySentinel = "SENTINEL-CREDENTIAL-5c1e07"

func sentinelOverride() agentcfg.Override {
	return agentcfg.Override{
		Env:        []string{"MATRIX_TOKEN=" + displaySentinel, "PLAIN=1"},
		AppendArgs: []string{"--debug"},
	}
}

func sentinelAgentConfig() agentcfg.Config {
	return agentcfg.Config{
		Command:         "agent-binary",
		Env:             []string{"MATRIX_TOKEN=" + displaySentinel},
		Headers:         map[string]string{"Authorization": displaySentinel},
		HealthcheckPath: "/health",
	}
}

// printed drives one reporting command the way the command builds its output:
// the same function, then the same MarshalIndent the command writes to stdout.
func printed(t *testing.T, payload map[string]any) string {
	t.Helper()
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	return string(out)
}

func agentShowReport(t *testing.T, reveal bool) string {
	t.Helper()
	payload := map[string]any{"agent_id": "agent-sentinel"}
	if err := addAgentConfigReport(payload, sentinelAgentConfig(), sentinelOverride(), reveal); err != nil {
		t.Fatalf("addAgentConfigReport: %v", err)
	}
	return printed(t, payload)
}

func TestNoAgentReportPrintsACredentialUnlessItWasAskedFor(t *testing.T) {
	overrideReport, err := agentOverrideReport("agent-sentinel", sentinelOverride(), false)
	if err != nil {
		t.Fatalf("agentOverrideReport: %v", err)
	}
	reports := map[string]string{
		"agent show":          agentShowReport(t, false),
		"agent override show": printed(t, overrideReport),
		"agent env list":      strings.Join(agentEnvLines(sentinelOverride().Env, false), "\n"),
	}
	for surface, report := range reports {
		if strings.Contains(report, displaySentinel) {
			t.Fatalf("%s stampa la credenziale senza che nessuno l'abbia chiesta:\n%s", surface, report)
		}
	}
	// Naming what it hides is the point: a report that says nothing would pass
	// the check above by saying nothing at all.
	for _, want := range []string{"MATRIX_TOKEN", "Authorization", "env_count", "header_count"} {
		if !strings.Contains(reports["agent show"], want) {
			t.Fatalf("agent show non nomina %q:\n%s", want, reports["agent show"])
		}
	}
	if !strings.Contains(reports["agent env list"], "MATRIX_TOKEN") || !strings.Contains(reports["agent env list"], "PLAIN") {
		t.Fatalf("agent env list non elenca i nomi:\n%s", reports["agent env list"])
	}
}

func TestTheAgentReportsPrintTheValuesWhenTheOperatorAsksForThem(t *testing.T) {
	overrideReport, err := agentOverrideReport("agent-sentinel", sentinelOverride(), true)
	if err != nil {
		t.Fatalf("agentOverrideReport: %v", err)
	}
	reports := map[string]string{
		"agent show":          agentShowReport(t, true),
		"agent override show": printed(t, overrideReport),
		"agent env list":      strings.Join(agentEnvLines(sentinelOverride().Env, true), "\n"),
	}
	for surface, report := range reports {
		if !strings.Contains(report, displaySentinel) {
			t.Fatalf("%s non stampa il valore che l'operatore ha chiesto:\n%s", surface, report)
		}
	}
}
