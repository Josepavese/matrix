package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/spf13/cobra"
)

// vaultTestStore is the read-only vault these tests read. Opening the real one
// needs a bolt file behind a broker; none of the behaviour under test needs the
// file, only the records.
type vaultTestStore struct {
	values map[string][]byte
	closed bool
}

func (s *vaultTestStore) Get(key string) ([]byte, error) { return s.values[key], nil }
func (s *vaultTestStore) Set(string, []byte) error       { return nil }
func (s *vaultTestStore) Delete(string) error            { return nil }
func (s *vaultTestStore) Close() error                   { s.closed = true; return nil }

func (s *vaultTestStore) List(prefix string) ([]string, error) {
	keys := make([]string, 0, len(s.values))
	for key := range s.values {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// vaultRunRecord builds the record the run lifecycle writes, through the same
// type the writer marshals, so a change to the record's shape reaches these
// tests instead of passing them.
func vaultRunRecord(t *testing.T, mutate func(*runtrace.Run)) []byte {
	t.Helper()
	run := runtrace.Run{
		ID:            "run-1",
		ChannelID:     "channel-1",
		ExecutionMode: "run",
		Status:        "completed",
		StopReason:    "end_turn",
		Output:        "the terminal summary",
		OutputRef:     "matrix://runs/run-1/outcome",
	}
	if mutate != nil {
		mutate(&run)
	}
	blob, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshalling the run record: %v", err)
	}
	return blob
}

func useVaultStore(t *testing.T, store vaultReadStore) {
	t.Helper()
	previous := openVaultReadStore
	openVaultReadStore = func(string) (vaultReadStore, error) { return store, nil }
	t.Cleanup(func() { openVaultReadStore = previous })
}

func useVaultGetFlags(t *testing.T, field string, maxBytes int) {
	t.Helper()
	previousField, previousMax, previousReveal := vaultGetField, vaultGetMaxBytes, vaultGetReveal
	vaultGetField, vaultGetMaxBytes, vaultGetReveal = field, maxBytes, false
	t.Cleanup(func() { vaultGetField, vaultGetMaxBytes, vaultGetReveal = previousField, previousMax, previousReveal })
}

func useVaultSummaryCap(t *testing.T, maxBytes int) {
	t.Helper()
	previous := vaultSummaryMaxBytes
	vaultSummaryMaxBytes = maxBytes
	t.Cleanup(func() { vaultSummaryMaxBytes = previous })
}

func runVaultCommand(t *testing.T, run func(*cobra.Command) error) (string, error) {
	t.Helper()
	command := &cobra.Command{}
	var out bytes.Buffer
	command.SetOut(&out)
	err := run(command)
	return out.String(), err
}

// TestVaultSummaryPrintsOnlyTheTerminalSummary is the P1 contract: the summary
// getter answers with the terminal content and nothing else — no status line, no
// record envelope, no diagnostic fields.
func TestVaultSummaryPrintsOnlyTheTerminalSummary(t *testing.T) {
	useVaultStore(t, &vaultTestStore{values: map[string][]byte{
		"runtrace.run.run-1": vaultRunRecord(t, nil),
	}})
	useVaultSummaryCap(t, defaultVaultGetMaxBytes)

	out, err := runVaultCommand(t, func(cmd *cobra.Command) error { return runVaultSummary(cmd, "run-1") })
	if err != nil {
		t.Fatalf("summary of a completed run: %v", err)
	}
	if out != "the terminal summary\n" {
		t.Fatalf("summary printed %q, want only the terminal summary", out)
	}
}

// TestVaultSummaryRefusesASummaryOverTheCap pins the size boundary: over the cap
// the answer is the dedicated code, and the oversized content is not printed, not
// truncated and not silently shortened.
func TestVaultSummaryRefusesASummaryOverTheCap(t *testing.T) {
	useVaultStore(t, &vaultTestStore{values: map[string][]byte{
		"runtrace.run.run-1": vaultRunRecord(t, func(run *runtrace.Run) {
			run.Output = strings.Repeat("x", 64)
		}),
	}})
	useVaultSummaryCap(t, 32)

	out, err := runVaultCommand(t, func(cmd *cobra.Command) error { return runVaultSummary(cmd, "run-1") })
	if err == nil {
		t.Fatalf("a summary over the cap must be refused, got %q", out)
	}
	for _, want := range []string{"ERR_VAULT_SUMMARY_TOO_LARGE", "64 bytes", "over the 32 byte cap"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal must name %q, got %q", want, err)
		}
	}
	if out != "" {
		t.Fatalf("a refused summary must not print content, got %q", out)
	}
}

// TestVaultSummaryWithoutATerminalPhaseIsRefused keeps an unfinished run from
// answering with the empty string, which a caller cannot tell from an empty
// summary.
func TestVaultSummaryWithoutATerminalPhaseIsRefused(t *testing.T) {
	useVaultStore(t, &vaultTestStore{values: map[string][]byte{
		"runtrace.run.run-1": vaultRunRecord(t, func(run *runtrace.Run) {
			run.Status, run.Output, run.OutputRef = "running", "", ""
		}),
	}})
	useVaultSummaryCap(t, defaultVaultGetMaxBytes)

	if _, err := runVaultCommand(t, func(cmd *cobra.Command) error { return runVaultSummary(cmd, "run-1") }); err == nil {
		t.Fatal("a run without a terminal summary must be refused")
	} else if !strings.Contains(err.Error(), "ERR_VAULT_SUMMARY_UNAVAILABLE") {
		t.Fatalf("refusal must carry its own code, got %q", err)
	}
}

// TestVaultResultIsDiagnosticAndLeavesTheSummaryToItsGetter is the other half of
// the separation: the result answers with the terminal outcome, the reference the
// summary lives at, and no summary content — a diagnostic answer is not a dump.
func TestVaultResultIsDiagnosticAndLeavesTheSummaryToItsGetter(t *testing.T) {
	useVaultStore(t, &vaultTestStore{values: map[string][]byte{
		"runtrace.run.run-1": vaultRunRecord(t, nil),
	}})

	out, err := runVaultCommand(t, func(cmd *cobra.Command) error { return runVaultResult(cmd, "run-1") })
	if err != nil {
		t.Fatalf("result of a completed run: %v", err)
	}
	for _, want := range []string{`"status": "completed"`, `"stop_reason": "end_turn"`, `"summary_ref": "matrix://runs/run-1/outcome"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("diagnostic result must carry %s, got %s", want, out)
		}
	}
	if strings.Contains(out, "the terminal summary") || strings.Contains(out, `"summary":`) {
		t.Fatalf("result must not carry the summary content: %s", out)
	}
}

// TestVaultGetListsRecordFieldsBeforeAnyParseError is EP-04.C: asking for help on
// a typed key answers with the fields and their types, so the caller's next call
// is a field access instead of another parse failure. The command is driven the
// way the operator drives it, through the root command and --help.
func TestVaultGetListsRecordFieldsBeforeAnyParseError(t *testing.T) {
	var out bytes.Buffer
	previousOut := rootCmd.OutOrStdout()
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"vault", "get", "runtrace.run.run-1", "--help"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(previousOut)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("--help on a typed record: %v", err)
	}
	answer := out.String()
	for _, want := range []string{"typed record", "--field", "output", "string", "trace_policy", "object", "started_at"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("the help must name %q, got:\n%s", want, answer)
		}
	}
	if strings.Contains(answer, "not found") || strings.Contains(answer, "ERR_VAULT") {
		t.Fatalf("help must answer before any parse of the record, got:\n%s", answer)
	}
}

// TestVaultGetReadsOneFieldOfATypedRecord is the per-field access the help
// promises: a string field prints its text, a structured field prints its json.
func TestVaultGetReadsOneFieldOfATypedRecord(t *testing.T) {
	useVaultStore(t, &vaultTestStore{values: map[string][]byte{
		"runtrace.run.run-1": vaultRunRecord(t, func(run *runtrace.Run) {
			run.ClientMeta = map[string]interface{}{"origin": "test"}
		}),
	}})

	useVaultGetFlags(t, "output", defaultVaultGetMaxBytes)
	out, err := runVaultCommand(t, func(cmd *cobra.Command) error { return runVaultGet(cmd, "runtrace.run.run-1") })
	if err != nil {
		t.Fatalf("reading the output field: %v", err)
	}
	if out != "the terminal summary\n" {
		t.Fatalf("string field printed %q", out)
	}

	useVaultGetFlags(t, "client_meta", defaultVaultGetMaxBytes)
	out, err = runVaultCommand(t, func(cmd *cobra.Command) error { return runVaultGet(cmd, "runtrace.run.run-1") })
	if err != nil {
		t.Fatalf("reading a structured field: %v", err)
	}
	if !strings.Contains(out, `"origin":"test"`) {
		t.Fatalf("structured field printed %q, want its json", out)
	}
}

// TestVaultGetNamesATypedRecordInsteadOfFailingAParse answers the reported
// symptom: a string getter on a typed record says what the value is and which
// field list to ask for, instead of reporting a parse failure of the vault.
func TestVaultGetNamesATypedRecordInsteadOfFailingAParse(t *testing.T) {
	useVaultStore(t, &vaultTestStore{values: map[string][]byte{
		"runtrace.run.run-1": vaultRunRecord(t, nil),
	}})
	useVaultGetFlags(t, "", defaultVaultGetMaxBytes)

	_, err := runVaultCommand(t, func(cmd *cobra.Command) error { return runVaultGet(cmd, "runtrace.run.run-1") })
	if err == nil {
		t.Fatal("a string getter on a typed record must refuse, not print the record")
	}
	for _, want := range []string{"ERR_VAULT_TYPED_RECORD", "--help", "--field"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal must name %q, got %q", want, err)
		}
	}
}

// TestVaultGetFieldRefusesAValueOverTheCap is EP-04.E: the per-field access is
// bounded by the same explicit cap, and says which cap refused it.
func TestVaultGetFieldRefusesAValueOverTheCap(t *testing.T) {
	useVaultStore(t, &vaultTestStore{values: map[string][]byte{
		"runtrace.run.run-1": vaultRunRecord(t, func(run *runtrace.Run) {
			run.Output = strings.Repeat("x", 64)
		}),
	}})
	useVaultGetFlags(t, "output", 32)

	out, err := runVaultCommand(t, func(cmd *cobra.Command) error { return runVaultGet(cmd, "runtrace.run.run-1") })
	if err == nil {
		t.Fatalf("a field over the cap must be refused, got %q", out)
	}
	if !strings.Contains(err.Error(), "ERR_VAULT_FIELD_TOO_LARGE") {
		t.Fatalf("refusal must carry its own code, got %q", err)
	}
}

// TestVaultGetStillReadsStringKeys guards the behaviour the getter exists for: a
// string key prints its text, redacted when the key names a secret.
func TestVaultGetStillReadsStringKeys(t *testing.T) {
	useVaultStore(t, &vaultTestStore{values: map[string][]byte{
		`system.configured`: []byte(`"true"`),
	}})
	useVaultGetFlags(t, "", defaultVaultGetMaxBytes)

	out, err := runVaultCommand(t, func(cmd *cobra.Command) error { return runVaultGet(cmd, "system.configured") })
	if err != nil {
		t.Fatalf("reading a string key: %v", err)
	}
	if out != "true\n" {
		t.Fatalf("string key printed %q, want its text", out)
	}
}
