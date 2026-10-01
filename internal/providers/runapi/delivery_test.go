package runapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/deliverycontract"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

func writeWorkspaceFile(dir, name, content string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func seedContractedRun(t *testing.T, server *Server, runID, workspace, status string, contract *deliverycontract.Contract) {
	t.Helper()
	run := runtrace.Run{
		ID: runID, AgentID: "peer", ChannelID: "ch", Status: status,
		WorkspacePath: workspace, StartedAt: time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC),
	}
	if err := server.Store().SaveRun(run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if contract != nil {
		appendDeliveryDeclared(server.Store(), runID, *contract)
	}
}

func loadContractedRun(t *testing.T, server *Server, runID string) runtrace.Run {
	t.Helper()
	run, found, err := server.Store().LoadRun(runID)
	if err != nil || !found {
		t.Fatalf("LoadRun: found=%v err=%v", found, err)
	}
	return run
}

func getExplanation(t *testing.T, mux *http.ServeMux, runID string) map[string]interface{} {
	t.Helper()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, RunResourcePrefixV1+runID+"/explain", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("explain status = %d", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode explain: %v", err)
	}
	return body
}

// TestACompletedRunWithoutItsArtifactIsReportedAsIncompleteDelivery is the
// criterion the PM set. Both answers must appear, and they must be different
// answers: the protocol reported that the turn ended, and the delivery contract
// reports that what was asked for is not there. Before this, only the first was
// visible, which is how "completed" came to read as "the work arrived".
func TestACompletedRunWithoutItsArtifactIsReportedAsIncompleteDelivery(t *testing.T) {
	server, mux := traceExportServer(t)
	seedContractedRun(t, server, "run-contract", t.TempDir(), runtrace.StatusCompleted, &deliverycontract.Contract{
		Artifacts: []deliverycontract.Artifact{{Path: "REPORT.md"}},
	})
	server.recordDeliveryVerdict(loadContractedRun(t, server, "run-contract"))

	body := getExplanation(t, mux, "run-contract")
	if body["status"] != runtrace.StatusCompleted {
		t.Fatalf("protocol status = %v, want %q", body["status"], runtrace.StatusCompleted)
	}
	if body["acceptance_status"] != deliverycontract.StatusIncomplete {
		t.Fatalf("acceptance = %v, want %q", body["acceptance_status"], deliverycontract.StatusIncomplete)
	}
	delivery, ok := body["delivery"].(map[string]interface{})
	if !ok {
		t.Fatalf("the explanation carries no delivery detail: %v", body["delivery"])
	}
	checks, ok := delivery["checks"].([]interface{})
	if !ok || len(checks) != 1 {
		t.Fatalf("checks = %v, want the one declared requirement", delivery["checks"])
	}
	check, ok := checks[0].(map[string]interface{})
	if !ok {
		t.Fatalf("check = %v, want an object", checks[0])
	}
	if check["status"] != "missing" {
		t.Fatalf("check = %v, want the artifact reported missing", check)
	}
}

// TestARunWithoutAContractIsNotReportedAsAccepted is the honesty rule on the
// diagnostic surface: the absence of a contract is not evidence of delivery, and
// a run that never declared one must not come back looking accepted.
func TestARunWithoutAContractIsNotReportedAsAccepted(t *testing.T) {
	server, mux := traceExportServer(t)
	// An empty contract is the realistic shape of "the caller declared nothing":
	// the request carried the object and it requires nothing. It must be treated
	// as no contract at all, not as a contract that was satisfied.
	empty := deliverycontract.Contract{}
	seedContractedRun(t, server, "run-nocontract", t.TempDir(), runtrace.StatusCompleted, &empty)
	server.recordDeliveryVerdict(loadContractedRun(t, server, "run-nocontract"))

	body := getExplanation(t, mux, "run-nocontract")
	if body["acceptance_status"] != deliverycontract.StatusNotDeclared {
		t.Fatalf("acceptance = %v, want %q", body["acceptance_status"], deliverycontract.StatusNotDeclared)
	}
	if body["acceptance_status"] == deliverycontract.StatusAccepted {
		t.Fatal("a run with no contract was reported as an accepted delivery")
	}
	// Opt-in is explicit: a run that declared nothing must not carry a
	// declaration in its trace either, or the record would show a contract
	// nobody ever asked for.
	events, err := server.Store().LoadEvents("run-nocontract", 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	for _, event := range events {
		if event.Kind == deliverycontract.EventDeclared {
			t.Fatal("a run that declared no contract recorded a delivery declaration")
		}
	}
}

// TestTheVerdictIsDecidedOnceAndSurvivesTheWorkspaceChanging pins the reason the
// verdict is stored rather than recomputed: it must keep saying what was true
// when the run ended, even if someone produces the artifact afterwards.
func TestTheVerdictIsDecidedOnceAndSurvivesTheWorkspaceChanging(t *testing.T) {
	server, mux := traceExportServer(t)
	workspace := t.TempDir()
	seedContractedRun(t, server, "run-once", workspace, runtrace.StatusCompleted, &deliverycontract.Contract{
		Artifacts: []deliverycontract.Artifact{{Path: "REPORT.md"}},
	})
	server.recordDeliveryVerdict(loadContractedRun(t, server, "run-once"))

	if err := writeWorkspaceFile(workspace, "REPORT.md", "# written after the run"); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	// A second terminal path must not re-decide a contract that was settled.
	server.recordDeliveryVerdict(loadContractedRun(t, server, "run-once"))

	body := getExplanation(t, mux, "run-once")
	if body["acceptance_status"] != deliverycontract.StatusIncomplete {
		t.Fatalf("acceptance = %v, want the verdict as it stood when the run ended", body["acceptance_status"])
	}
	events, err := server.Store().LoadEvents("run-once", 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	verdicts := 0
	for _, event := range events {
		if event.Kind == deliverycontract.EventVerified {
			verdicts++
		}
	}
	if verdicts != 1 {
		t.Fatalf("the run recorded %d delivery verdicts, want exactly one: two answers to one question leave a reader taking the wrong one", verdicts)
	}
}

// TestADeclaredContractTheRunNeverEvaluatedIsUnverifiable separates "the work is
// missing" from "Matrix never got to look". A run that ended before its contract
// was settled must not be reported as either accepted or incomplete.
func TestADeclaredContractTheRunNeverEvaluatedIsUnverifiable(t *testing.T) {
	server, mux := traceExportServer(t)
	seedContractedRun(t, server, "run-unevaluated", t.TempDir(), runtrace.StatusFailed, &deliverycontract.Contract{
		Artifacts: []deliverycontract.Artifact{{Path: "REPORT.md"}},
	})

	body := getExplanation(t, mux, "run-unevaluated")
	if body["acceptance_status"] != deliverycontract.StatusUnverifiable {
		t.Fatalf("acceptance = %v, want %q", body["acceptance_status"], deliverycontract.StatusUnverifiable)
	}
	delivery, _ := body["delivery"].(map[string]interface{})
	if delivery == nil || delivery["reason"] == "" {
		t.Fatalf("an unevaluated contract must say why: %v", body["delivery"])
	}
}

// TestTheVerdictSurvivesARedactingTracePolicy guards the reason the verdict is
// written to the event's status and not only into its metadata: a trace policy
// may redact metadata, and the acceptance answer must not disappear with it.
func TestTheVerdictSurvivesARedactingTracePolicy(t *testing.T) {
	server, _ := traceExportServer(t)
	runID := "run-redacted"
	seedContractedRun(t, server, runID, t.TempDir(), runtrace.StatusCompleted, &deliverycontract.Contract{
		Artifacts: []deliverycontract.Artifact{{Path: "REPORT.md"}},
	})
	run := loadContractedRun(t, server, runID)
	run.TracePolicy = runtrace.TracePolicy{ContentMode: runtrace.ContentModeRedacted}
	if err := server.Store().SaveRun(run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	server.recordDeliveryVerdict(run)

	trace, found, err := server.Store().Trace(runID)
	if err != nil || !found {
		t.Fatalf("Trace: found=%v err=%v", found, err)
	}
	for _, event := range trace.Events {
		if event.Kind == deliverycontract.EventVerified {
			if event.Status != deliverycontract.StatusIncomplete {
				t.Fatalf("verified event status = %q, want %q", event.Status, deliverycontract.StatusIncomplete)
			}
			return
		}
	}
	t.Fatal("the exported trace lost the delivery verdict under a redacting policy")
}

// TestTheTerminalPathSettlesTheContract proves the hook, not just the function:
// without this, every test above would still pass if terminalResult never called
// the evaluation, and the whole feature would be dead code with green tests.
func TestTheTerminalPathSettlesTheContract(t *testing.T) {
	server, mux := traceExportServer(t)
	seedContractedRun(t, server, "run-hook", t.TempDir(), runtrace.StatusCompleted, &deliverycontract.Contract{
		Artifacts: []deliverycontract.Artifact{{Path: "REPORT.md"}},
	})
	server.terminalResult("run-hook", runExecutionResult{})

	body := getExplanation(t, mux, "run-hook")
	if body["acceptance_status"] != deliverycontract.StatusIncomplete {
		t.Fatalf("acceptance = %v, want the contract settled by the terminal path", body["acceptance_status"])
	}
}

// TestAContractIsRefusedBeforeTheRunExists covers the opt-in gate at the request
// boundary: a contract that cannot be evaluated must stop the run, not travel
// with it and be discovered later.
func TestAContractIsRefusedBeforeTheRunExists(t *testing.T) {
	valid := deliverycontract.Contract{Artifacts: []deliverycontract.Artifact{{Path: "REPORT.md"}}}
	empty := deliverycontract.Contract{}
	cases := []struct {
		name     string
		contract *deliverycontract.Contract
		declared bool
		wantErr  bool
	}{
		{name: "absent", contract: nil},
		{name: "declares nothing", contract: &empty},
		{name: "declared", contract: &valid, declared: true},
		{name: "shell string", contract: &deliverycontract.Contract{Validator: &deliverycontract.Validator{Command: []string{"git diff --quiet"}}}, wantErr: true},
		{name: "short digest", contract: &deliverycontract.Contract{Artifacts: []deliverycontract.Artifact{{Path: "a.md", SHA256: "deadbeef"}}}, wantErr: true},
		{name: "negative timeout", contract: &deliverycontract.Contract{Validator: &deliverycontract.Validator{Command: []string{"true"}, TimeoutSeconds: -1}}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := runRequest{DeliveryContract: tc.contract}
			contract, err := req.declaredContract()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("an unevaluable contract was accepted: %#v", contract)
				}
				return
			}
			if err != nil {
				t.Fatalf("declaredContract: %v", err)
			}
			if tc.declared != (contract != nil) {
				t.Fatalf("declared = %v, want %v", contract != nil, tc.declared)
			}
		})
	}
}

// TestTheRunRecordsTheContractBeforeItIsDispatched proves the call site, not the
// helper: without this, the append could be removed and every other test would
// still pass while no run ever carried a contract.
func TestTheRunRecordsTheContractBeforeItIsDispatched(t *testing.T) {
	server, _ := traceExportServer(t)
	contract := deliverycontract.Contract{Artifacts: []deliverycontract.Artifact{{Path: "REPORT.md"}}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, RunResourcePrefixV1, nil)
	run, ok := server.acceptNewRun(w, r, runRequest{
		ChannelID: "ch", DeliveryContract: &contract,
	}, "peer")
	if !ok {
		t.Fatalf("the run was not accepted: %d %s", w.Code, w.Body.String())
	}
	events, err := server.Store().LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	for _, event := range events {
		if event.Kind == deliverycontract.EventDeclared {
			var recorded deliverycontract.Contract
			if !deliverycontract.Decode(event.Metadata, &recorded) {
				t.Fatal("the declaration was recorded without its contract")
			}
			if len(recorded.Artifacts) != 1 || recorded.Artifacts[0].Path != "REPORT.md" {
				t.Fatalf("recorded contract = %#v", recorded)
			}
			return
		}
	}
	t.Fatal("the run was dispatched without recording the contract it must honour")
}

// TestAnUnevaluableContractStopsTheRunAtTheBoundary: the refusal has to happen
// before a run id is consumed, and it has to be a 400 rather than a run that
// fails later for reasons the caller cannot connect to their own request.
func TestAnUnevaluableContractStopsTheRunAtTheBoundary(t *testing.T) {
	server, _ := traceExportServer(t)
	bad := deliverycontract.Contract{Validator: &deliverycontract.Validator{Command: []string{"git diff --quiet"}}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, RunResourcePrefixV1, nil)
	if _, ok := server.acceptNewRun(w, r, runRequest{ChannelID: "ch", DeliveryContract: &bad}, "peer"); ok {
		t.Fatal("a run was accepted carrying an unevaluable contract")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "argv") {
		t.Fatalf("the refusal does not say what is wrong: %s", w.Body.String())
	}
}

// TestTheContractIsDecodedFromTheRequestJSON proves the field is actually wired
// to the wire: a contract expressed as a string instead of an object, and a
// validator expressed as a shell string, must both be refused with a 400 by the
// real route. Without this, a wrong json tag would leave every other test green
// while no caller could ever declare a contract.
func TestTheContractIsDecodedFromTheRequestJSON(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "contract is a string", body: `{"channel_id":"ch","input":"do it","delivery_contract":"REPORT.md"}`},
		{name: "validator is a shell string", body: `{"channel_id":"ch","input":"do it","delivery_contract":{"validator":{"command":"git diff --quiet"}}}`},
		{name: "digest is malformed", body: `{"channel_id":"ch","input":"do it","delivery_contract":{"artifacts":[{"path":"a.md","sha256":"nope"}]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			server := NewServer(&runTestRouter{}).WithTraceStorage(memstore.New())
			server.RegisterRoutes(mux)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, RunPathV1, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			mux.ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
	}
}

// TestAVerdictEventWhosePayloadIsGoneStillAnswersFromItsStatus covers the
// fallback branch: a trace policy can strip an event's metadata, and the verdict
// must then be answered from the event's own status rather than reported as a
// missing verdict. Without this branch, a redacted run reads as undecided.
func TestAVerdictEventWhosePayloadIsGoneStillAnswersFromItsStatus(t *testing.T) {
	contract := deliverycontract.Contract{Artifacts: []deliverycontract.Artifact{{Path: "REPORT.md"}}}
	events := []runtrace.Event{
		{Kind: deliverycontract.EventDeclared, Metadata: deliverycontract.Encode(contract)},
		// The payload did not survive: no metadata at all, only the status.
		{Kind: deliverycontract.EventVerified, Status: deliverycontract.StatusIncomplete},
	}
	status, verdict := deliveryExplanation(events)
	if status != deliverycontract.StatusIncomplete {
		t.Fatalf("acceptance = %q, want %q from the event status", status, deliverycontract.StatusIncomplete)
	}
	if verdict == nil || verdict.Status != deliverycontract.StatusIncomplete {
		t.Fatalf("verdict = %#v, want it answered from the status alone", verdict)
	}
}

// TestSimultaneousTerminalsRecordExactlyOneVerdict covers the race the guard
// alone did not: checking for an existing verdict and appending a new one are two
// steps, and a cancel racing a completion can reach both. Two verdicts on one
// question leave a reader taking whichever it finds first.
func TestSimultaneousTerminalsRecordExactlyOneVerdict(t *testing.T) {
	server, _ := traceExportServer(t)
	runID := "run-race"
	seedContractedRun(t, server, runID, t.TempDir(), runtrace.StatusCompleted, &deliverycontract.Contract{
		Artifacts: []deliverycontract.Artifact{{Path: "REPORT.md"}},
	})
	run := loadContractedRun(t, server, runID)

	const terminals = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < terminals; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			server.recordDeliveryVerdict(run)
		}()
	}
	close(start)
	wg.Wait()

	events, err := server.Store().LoadEvents(runID, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	verdicts := 0
	for _, event := range events {
		if event.Kind == deliverycontract.EventVerified {
			verdicts++
		}
	}
	if verdicts != 1 {
		t.Fatalf("%d simultaneous terminal paths recorded %d verdicts, want exactly one", terminals, verdicts)
	}
}
