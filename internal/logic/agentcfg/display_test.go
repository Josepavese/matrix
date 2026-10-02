package agentcfg

import (
	"encoding/json"
	"strings"
	"testing"
)

// Two credentials that exist nowhere else: if either shows up in a report, the
// report is carrying a value.
const (
	sentinelEnvValue    = "ENV-SENTINEL-4f2c9a"
	sentinelHeaderValue = "HEADER-SENTINEL-77bd31"
)

func sentinelConfig() Config {
	return Config{
		Command:         "agent-binary",
		Args:            []string{"--serve"},
		Env:             []string{"MATRIX_TOKEN=" + sentinelEnvValue, "PLAIN=1"},
		Headers:         map[string]string{"Authorization": sentinelHeaderValue},
		Kind:            "acp",
		Transport:       "stdio",
		HealthcheckPath: "/health",
	}
}

func TestTheDisplayOfAConfigurationNamesAndCountsAndCarriesNoValue(t *testing.T) {
	shown, err := DisplayConfig(sentinelConfig(), false)
	if err != nil {
		t.Fatalf("DisplayConfig: %v", err)
	}
	report := marshalReport(t, shown)
	if strings.Contains(report, sentinelEnvValue) || strings.Contains(report, sentinelHeaderValue) {
		t.Fatalf("il report di default porta un valore:\n%s", report)
	}
	if !strings.Contains(report, "MATRIX_TOKEN") || !strings.Contains(report, "Authorization") {
		t.Fatalf("il report non nomina quello che nasconde:\n%s", report)
	}
	if shown["env_count"] != 2 || shown["header_count"] != 1 {
		t.Fatalf("conteggi = env %v header %v, vuole 2 e 1", shown["env_count"], shown["header_count"])
	}
	if _, ok := shown["env"]; ok {
		t.Fatal("il report di default porta la lista env")
	}
	if _, ok := shown["headers"]; ok {
		t.Fatal("il report di default porta la mappa headers")
	}
	if shown["command"] != "agent-binary" || shown["kind"] != "acp" {
		t.Fatalf("il resto della configurazione non e' passato: %+v", shown)
	}
}

func TestTheValuesComeBackOnlyWhenTheyWereAskedFor(t *testing.T) {
	shown, err := DisplayConfig(sentinelConfig(), true)
	if err != nil {
		t.Fatalf("DisplayConfig: %v", err)
	}
	report := marshalReport(t, shown)
	if !strings.Contains(report, sentinelEnvValue) || !strings.Contains(report, sentinelHeaderValue) {
		t.Fatalf("il report richiesto non porta i valori:\n%s", report)
	}
	if !strings.Contains(report, "MATRIX_TOKEN") {
		t.Fatalf("il report richiesto ha perso i nomi:\n%s", report)
	}
}

func TestTheOverrideFollowsTheSameRule(t *testing.T) {
	override := Override{Env: []string{"MATRIX_TOKEN=" + sentinelEnvValue}, AppendArgs: []string{"--debug"}}
	quiet, err := DisplayOverride(override, false)
	if err != nil {
		t.Fatalf("DisplayOverride: %v", err)
	}
	if report := marshalReport(t, quiet); strings.Contains(report, sentinelEnvValue) {
		t.Fatalf("l'override di default porta un valore:\n%s", report)
	}
	loud, err := DisplayOverride(override, true)
	if err != nil {
		t.Fatalf("DisplayOverride: %v", err)
	}
	if report := marshalReport(t, loud); !strings.Contains(report, sentinelEnvValue) {
		t.Fatalf("l'override richiesto non porta il valore:\n%s", report)
	}
}

func TestAShownEnvironmentNameIsNeverAValue(t *testing.T) {
	names := EnvNames([]string{
		"ZED_TOKEN=" + sentinelEnvValue,
		"ALPHA=1",
		"ALPHA=2",
		"EMPTY=",
		"",
		"=senza-nome",
		"NOME-SENZA-VALORE",
	})
	want := []string{"ALPHA", "EMPTY", "NOME-SENZA-VALORE", "ZED_TOKEN"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("nomi = %v, vuole %v", names, want)
	}
	if strings.Contains(strings.Join(names, ","), sentinelEnvValue) {
		t.Fatal("un nome mostrato e' un valore")
	}
}

func TestTheEnvironmentEffectDeclaresNamesAndCount(t *testing.T) {
	shown := DisplayEnv([]string{"MATRIX_TOKEN=" + sentinelEnvValue}, false)
	if report := marshalReport(t, shown); strings.Contains(report, sentinelEnvValue) {
		t.Fatalf("l'effetto env di default porta un valore:\n%s", report)
	}
	if shown["env_count"] != 1 {
		t.Fatalf("env_count = %v, vuole 1", shown["env_count"])
	}
	revealed := DisplayEnv([]string{"MATRIX_TOKEN=" + sentinelEnvValue}, true)
	if report := marshalReport(t, revealed); !strings.Contains(report, sentinelEnvValue) {
		t.Fatalf("l'effetto env richiesto non porta il valore:\n%s", report)
	}
}

func marshalReport(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(encoded)
}
