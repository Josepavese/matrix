package runapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

// TestTraceExportCarriesTheStallView proves the visibility reaches the consumer
// through the export it already reads, not only through an in-process call.
// The original issue was raised by an operator who had to open a provider's
// private database to find out whether a run was still moving; a view that only
// exists inside the package would not have helped them.
func TestTraceExportCarriesTheStallView(t *testing.T) {
	server, mux := traceExportServer(t)
	base := time.Date(2026, 10, 1, 15, 43, 35, 0, time.UTC)
	seedRunWithEvents(t, server, "run-stall-export", runtrace.TracePolicy{ContentMode: runtrace.ContentModeRefs}, []runtrace.Event{
		traceEvent(runtrace.KindPromptSent, "ses-parent", 1, base),
		traceEvent(runtrace.KindToolCallRequested, "ses-parent", 2, base.Add(8*time.Second)),
		traceEvent(runtrace.KindToolCallRequested, "ses-child", 3, base.Add(20*time.Second)),
	})

	body := getTrace(t, mux, "run-stall-export")
	if body.Stall == nil {
		t.Fatal("the trace export carries no stall view")
	}
	if body.Stall.Waiting != runtrace.WaitToolResult {
		t.Fatalf("waiting = %q, want %q", body.Stall.Waiting, runtrace.WaitToolResult)
	}
	if len(body.Stall.Pending) != 2 {
		t.Fatalf("pending = %#v, want both unanswered tool calls", body.Stall.Pending)
	}
	if body.Stall.Pending[0].Name != "read_file" || body.Stall.Pending[0].SessionID != "ses-parent" {
		t.Fatalf("the export lost which tool is stuck: %#v", body.Stall.Pending[0])
	}
	if len(body.Stall.Sessions) != 2 || body.Stall.Sessions[1].SessionID != "ses-child" {
		t.Fatalf("the export lost the parent/child split: %#v", body.Stall.Sessions)
	}
}

// TestTraceExportKeepsStallVisibilityUnderTheOperatorPolicy is the regression
// guard for the failure mode this feature is most likely to have: the stall view
// is derived from protocol metadata and tool names, both of which the trace
// policy strips for runs that do not opt in. Computing the view after the policy
// would leave it correct in tests and blind on an operator's trace.
func TestTraceExportKeepsStallVisibilityUnderTheOperatorPolicy(t *testing.T) {
	server, mux := traceExportServer(t)
	base := time.Date(2026, 10, 1, 19, 13, 31, 0, time.UTC)
	seedRunWithEvents(t, server, "run-stall-refs", runtrace.TracePolicy{ContentMode: runtrace.ContentModeRefs}, []runtrace.Event{
		traceEvent(runtrace.KindToolCallRequested, "ses-child", 5, base),
	})

	body := getTrace(t, mux, "run-stall-refs")
	if body.Stall == nil {
		t.Fatal("the trace export carries no stall view")
	}
	// The run's own session is reported even though no event named it: a session
	// that produced nothing is the finding, not a reason to leave it out.
	if len(body.Stall.Sessions) != 2 {
		t.Fatalf("sessions = %#v, want the silent run session and the peer's session", body.Stall.Sessions)
	}
	if !body.Stall.Sessions[0].RunSession || body.Stall.Sessions[0].LastActivity != nil {
		t.Fatalf("run session = %#v, want it marked and reported as silent", body.Stall.Sessions[0])
	}
	if body.Stall.Sessions[1].SessionID != "ses-child" || body.Stall.Sessions[1].RunSession {
		t.Fatalf("session attribution did not survive the trace policy: %#v", body.Stall.Sessions[1])
	}
	if len(body.Stall.Pending) != 1 || body.Stall.Pending[0].Name != "read_file" {
		t.Fatalf("the pending request did not survive the trace policy: %#v", body.Stall.Pending)
	}
	// The policy must still be doing its job on the exported events themselves,
	// otherwise this test would pass for the wrong reason.
	if len(body.Events) == 0 || body.Events[0].ProtocolMeta != nil {
		t.Fatal("this test needs a policy that strips protocol metadata from exported events")
	}
}

func traceExportServer(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()
	server := NewServer(&runTestRouter{}).WithTraceStorage(memstore.New())
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	return server, mux
}

func traceEvent(kind, session string, sequence int, at time.Time) runtrace.Event {
	event := runtrace.Event{
		Kind: kind, Actor: "agent", Sequence: sequence, Timestamp: at,
		Status:       runtrace.StatusRunning,
		ProtocolMeta: map[string]interface{}{"session_id": session},
	}
	if kind == runtrace.KindToolCallRequested {
		event.ToolCallID = "call-" + session
		event.ToolName = "read_file"
	}
	return event
}

func seedRunWithEvents(t *testing.T, server *Server, runID string, policy runtrace.TracePolicy, events []runtrace.Event) {
	t.Helper()
	run := runtrace.Run{
		ID: runID, AgentID: "peer", ChannelID: "ch", ExecutionMode: runtrace.ExecutionModeAsync,
		Status: runtrace.StatusRunning, RemoteSessionID: "ses-parent", TracePolicy: policy,
		StartedAt: time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC),
	}
	if err := server.Store().SaveRun(run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for _, event := range events {
		event.RunID = runID
		if _, err := server.Store().AppendEvent(event); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
}

func getTrace(t *testing.T, mux *http.ServeMux, runID string) runtrace.Trace {
	t.Helper()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, RunResourcePrefixV1+runID+"/trace", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET trace: %d %s", w.Code, w.Body.String())
	}
	var body runtrace.Trace
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode trace: %v", err)
	}
	return body
}

func elicitationNotification(t *testing.T, server *Server, runID, kind string) (runtrace.Notification, bool) {
	t.Helper()
	items, _, err := server.Store().LoadNotificationsAfter(0, 100, map[string]struct{}{runID: {}})
	if err != nil {
		t.Fatalf("LoadNotificationsAfter: %v", err)
	}
	for _, item := range items {
		if item.Kind == kind {
			return item, true
		}
	}
	return runtrace.Notification{}, false
}

func waitForNotification(t *testing.T, server *Server, runID, kind string) runtrace.Notification {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if item, found := elicitationNotification(t, server, runID, kind); found {
			return item
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("notification %s never recorded for %s", kind, runID)
	return runtrace.Notification{}
}

func waitForOpenElicitation(t *testing.T, service *elicitation.Service) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pending := service.Pending(); len(pending) > 0 {
			return pending[0].Request.ID
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the elicitation was never registered")
	return ""
}

// TestElicitationLifecycleReachesTheRunTrace is the end-to-end proof for the gap
// this change closes. Before it, runapi recorded only the opening: a run blocked
// on a human answer looked identical to a run whose peer had gone quiet, and the
// only way to tell was to open the operator's own database. The test drives the
// real service, the real subscriber, the real store and the real export.
func TestElicitationLifecycleReachesTheRunTrace(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	server, mux := traceExportServer(t)
	server.WithElicitationService(service)
	seedRunWithEvents(t, server, "run-elicitation", runtrace.TracePolicy{ContentMode: runtrace.ContentModeRefs}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = service.Ask(ctx, middleware.ElicitationRequest{
			AgentID: "peer", SessionID: "ses-parent",
			Mode: middleware.ElicitationModeForm, Message: "approve the write?",
		})
	}()
	id := waitForOpenElicitation(t, service)

	// The producer must write the kinds the stall vocabulary names, or the view
	// would silently stop seeing approvals. This is the coupling guard.
	opened := waitForNotification(t, server, "run-elicitation", runtrace.KindElicitationOpened)
	if opened.ElicitationID != id || opened.SessionID != "ses-parent" || opened.RunID != "run-elicitation" {
		t.Fatalf("opened notification = %#v", opened)
	}
	if opened.Timestamp.IsZero() {
		t.Fatal("the opened notification carries no timestamp, so the wait cannot be aged")
	}

	blocked := getTrace(t, mux, "run-elicitation")
	if blocked.Stall == nil || blocked.Stall.Waiting != runtrace.WaitElicitation {
		t.Fatalf("a run blocked on a human answer reports %#v", blocked.Stall)
	}
	if len(blocked.Stall.Pending) != 1 || blocked.Stall.Pending[0].ID != id {
		t.Fatalf("pending = %#v, want the open elicitation %s", blocked.Stall.Pending, id)
	}

	if !service.Respond(id, middleware.AcceptElicitation(nil)) {
		t.Fatal("the elicitation could not be answered")
	}
	waitForNotification(t, server, "run-elicitation", runtrace.KindElicitationResolved)

	answered := getTrace(t, mux, "run-elicitation")
	if answered.Stall == nil {
		t.Fatal("the trace carries no stall view")
	}
	if answered.Stall.Waiting == runtrace.WaitElicitation || len(answered.Stall.Pending) != 0 {
		t.Fatalf("the run is still reported as blocked after the answer: %#v", answered.Stall)
	}
}
