package runtrace

import (
	"testing"
	"time"
)

var stallBase = time.Date(2026, 10, 1, 15, 43, 35, 0, time.UTC)

func at(seconds int) time.Time { return stallBase.Add(time.Duration(seconds) * time.Second) }

func sessionEvent(kind, session string, seq, seconds int) Event {
	return Event{
		Kind: kind, Actor: "agent", Sequence: seq, Timestamp: at(seconds),
		ProtocolMeta: map[string]interface{}{sessionMetaKey: session},
	}
}

func toolRequested(id, name, session string, seq, seconds int) Event {
	event := sessionEvent(KindToolCallRequested, session, seq, seconds)
	event.ToolCallID = id
	event.ToolName = name
	event.Status = StatusRunning
	return event
}

func toolReceived(id, name, session string, seq, seconds int) Event {
	event := sessionEvent(KindToolResultReceived, session, seq, seconds)
	event.ToolCallID = id
	event.ToolName = name
	event.Status = StatusCompleted
	return event
}

func permissionRequested(id, session string, seq, seconds int) Event {
	event := sessionEvent(KindPermissionRequested, session, seq, seconds)
	event.PermissionID = id
	event.Summary = "write access"
	return event
}

func permissionResolved(id, session string, seq, seconds int) Event {
	event := sessionEvent(KindPermissionResolved, session, seq, seconds)
	event.PermissionID = id
	return event
}

func stalledRun(status string) Run {
	return Run{
		ID: "run-stall", AgentID: "peer", Status: status,
		RemoteSessionID: "ses-parent",
		TracePolicy:     TracePolicy{ContentMode: ContentModeRefs},
		StartedAt:       stallBase,
	}
}

// TestStallViewNamesTheWaitAndWhenItStarted is the core of the issue: a run
// whose tool call was never answered must say so, with the age of the wait,
// instead of leaving the consumer to read a provider database.
func TestStallViewNamesTheWaitAndWhenItStarted(t *testing.T) {
	events := []Event{
		sessionEvent(KindPromptSent, "ses-parent", 1, 0),
		toolRequested("call-1", "read_file", "ses-parent", 2, 10),
	}
	view := ObserveStall(stalledRun(StatusRunning), events, nil)
	if view.Waiting != WaitToolResult {
		t.Fatalf("waiting = %q, want %q", view.Waiting, WaitToolResult)
	}
	if !view.WaitingSince.Equal(at(10)) {
		t.Fatalf("waiting since = %s, want %s", view.WaitingSince, at(10))
	}
	if len(view.Pending) != 1 {
		t.Fatalf("pending = %#v, want the one unanswered tool call", view.Pending)
	}
	if got := view.Pending[0]; got.Kind != WaitToolResult || got.ID != "call-1" || got.Name != "read_file" {
		t.Fatalf("pending request = %#v", got)
	}
}

// TestStallViewStopsWaitingWhenTheAnswerArrives proves the view tracks
// resolutions rather than accumulating requests forever: an answered call is no
// longer a wait, and the run falls back to waiting on the peer.
func TestStallViewStopsWaitingWhenTheAnswerArrives(t *testing.T) {
	events := []Event{
		toolRequested("call-1", "read_file", "ses-parent", 1, 10),
		toolReceived("call-1", "read_file", "ses-parent", 2, 30),
	}
	view := ObserveStall(stalledRun(StatusRunning), events, nil)
	if len(view.Pending) != 0 {
		t.Fatalf("an answered call is still pending: %#v", view.Pending)
	}
	if view.Waiting != WaitProviderTurn {
		t.Fatalf("waiting = %q, want %q", view.Waiting, WaitProviderTurn)
	}
	if !view.WaitingSince.Equal(at(30)) {
		t.Fatalf("waiting since = %s, want the last observed activity %s", view.WaitingSince, at(30))
	}
}

// TestStallViewReportsAPendingPermission covers the second half of the issue's
// "pending request/tool/permission": an approval nobody answered is a wait of a
// different kind, and it must be named as such.
func TestStallViewReportsAPendingPermission(t *testing.T) {
	events := []Event{
		permissionRequested("perm-1", "ses-parent", 1, 25),
	}
	view := ObserveStall(stalledRun(StatusRunning), events, nil)
	if view.Waiting != WaitPermission {
		t.Fatalf("waiting = %q, want %q", view.Waiting, WaitPermission)
	}
	if len(view.Pending) != 1 || view.Pending[0].ID != "perm-1" {
		t.Fatalf("pending = %#v", view.Pending)
	}
	resolved := ObserveStall(stalledRun(StatusRunning), append(events, permissionResolved("perm-1", "ses-parent", 2, 26)), nil)
	if len(resolved.Pending) != 0 {
		t.Fatalf("a resolved permission is still pending: %#v", resolved.Pending)
	}
}

// TestStallViewReportsTheLongestOutstandingWait pins the choice of "current
// wait": the request that has been open longest is the one the run stopped being
// able to progress on, while a nested approval stays visible in the list.
func TestStallViewReportsTheLongestOutstandingWait(t *testing.T) {
	events := []Event{
		toolRequested("call-1", "read_file", "ses-parent", 1, 10),
		permissionRequested("perm-1", "ses-parent", 2, 25),
	}
	view := ObserveStall(stalledRun(StatusRunning), events, nil)
	if view.Waiting != WaitToolResult {
		t.Fatalf("waiting = %q, want the longest outstanding wait %q", view.Waiting, WaitToolResult)
	}
	if len(view.Pending) != 2 || view.Pending[0].ID != "call-1" || view.Pending[1].ID != "perm-1" {
		t.Fatalf("pending must be oldest first and complete: %#v", view.Pending)
	}
}

// TestStallViewSeparatesTheRunSessionFromSessionsThePeerOpened is the parent and
// child requirement, derived structurally: the session the run record names is
// the run's own, every other session observed in the trace was opened by the
// peer while the run was in flight. No agent or provider name is consulted.
func TestStallViewSeparatesTheRunSessionFromSessionsThePeerOpened(t *testing.T) {
	events := []Event{
		toolRequested("call-parent", "read_file", "ses-parent", 1, 10),
		toolRequested("call-child", "grep", "ses-child", 2, 20),
	}
	view := ObserveStall(stalledRun(StatusRunning), events, nil)
	if len(view.Sessions) != 2 {
		t.Fatalf("sessions = %#v, want the run session and the session the peer opened", view.Sessions)
	}
	parent, child := view.Sessions[0], view.Sessions[1]
	if parent.SessionID != "ses-parent" || !parent.RunSession {
		t.Fatalf("first session = %#v, want the run's session marked as such", parent)
	}
	if child.SessionID != "ses-child" || child.RunSession {
		t.Fatalf("second session = %#v, want the peer's session not marked as the run's", child)
	}
	if len(parent.Pending) != 1 || len(child.Pending) != 1 {
		t.Fatalf("pending requests were not kept per session: parent=%#v child=%#v", parent.Pending, child.Pending)
	}
	if child.LastActivity == nil || child.LastActivity.Kind != KindToolCallRequested {
		t.Fatalf("child last activity = %#v", child.LastActivity)
	}
}

// TestStallViewKeepsTheRunSessionVisibleWhileItIsSilent covers the case the
// operator actually hits: the session Matrix is attached to has produced nothing,
// and that absence is the finding, so the session must not be missing from the
// picture just because it has no events.
func TestStallViewKeepsTheRunSessionVisibleWhileItIsSilent(t *testing.T) {
	view := ObserveStall(stalledRun(StatusRunning), nil, nil)
	if len(view.Sessions) != 1 || view.Sessions[0].SessionID != "ses-parent" {
		t.Fatalf("sessions = %#v, want the silent run session", view.Sessions)
	}
	if view.Sessions[0].LastActivity != nil {
		t.Fatalf("a session with no events must report no activity: %#v", view.Sessions[0].LastActivity)
	}
	if view.Sessions[0].Waiting != WaitUnknown {
		t.Fatalf("waiting = %q, want %q", view.Sessions[0].Waiting, WaitUnknown)
	}
}

// TestStallViewDoesNotCallMatrixBookkeepingActivity reuses the completion
// evidence rule: routing, model selection, prompt and session events are things
// Matrix did, not things the peer did, and a stall view that treated them as
// progress would hide the exact stall the issue reports.
func TestStallViewDoesNotCallMatrixBookkeepingActivity(t *testing.T) {
	events := []Event{
		{Kind: "run.started", Actor: "matrix", Sequence: 1, Timestamp: at(0)},
		{Kind: KindPromptSent, Actor: "matrix", Sequence: 2, Timestamp: at(1)},
		{Kind: "routing.decision", Actor: "matrix", Sequence: 3, Timestamp: at(2)},
		{Kind: "model.selection", Actor: "matrix", Sequence: 4, Timestamp: at(3)},
	}
	view := ObserveStall(stalledRun(StatusRunning), events, nil)
	if view.LastActivity != nil {
		t.Fatalf("Matrix bookkeeping was reported as agent activity: %#v", view.LastActivity)
	}
}

// TestStallViewRecordsTheLastObservedActivity proves the field the issue asks
// for first: what moved last, and when, so a consumer can age the stall without
// opening a provider database.
func TestStallViewRecordsTheLastObservedActivity(t *testing.T) {
	events := []Event{
		toolRequested("call-1", "read_file", "ses-parent", 1, 10),
		toolReceived("call-1", "read_file", "ses-parent", 2, 42),
	}
	view := ObserveStall(stalledRun(StatusRunning), events, nil)
	if view.LastActivity == nil {
		t.Fatal("no activity observed")
	}
	if view.LastActivity.Kind != KindToolResultReceived || !view.LastActivity.Timestamp.Equal(at(42)) {
		t.Fatalf("last activity = %#v, want the tool result at %s", view.LastActivity, at(42))
	}
	if view.LastActivity.ToolName != "read_file" {
		t.Fatalf("last activity lost the tool it names: %#v", view.LastActivity)
	}
}

// TestStallViewSaysUnknownRatherThanGuessing is the honesty rule for this
// feature: a live run with nothing outstanding and nothing observed is
// "unknown", not "waiting on the provider". A consumer must be able to tell
// "Matrix has no evidence" from "Matrix has evidence of waiting".
func TestStallViewSaysUnknownRatherThanGuessing(t *testing.T) {
	if got := ObserveStall(stalledRun(StatusRunning), nil, nil).Waiting; got != WaitUnknown {
		t.Fatalf("waiting = %q, want %q for a run with no evidence at all", got, WaitUnknown)
	}
	withPrompt := ObserveStall(stalledRun(StatusRunning), []Event{sessionEvent(KindPromptSent, "ses-parent", 1, 0)}, nil)
	if withPrompt.Waiting != WaitProviderTurn {
		t.Fatalf("waiting = %q, want %q once a prompt was sent", withPrompt.Waiting, WaitProviderTurn)
	}
}

// TestStallViewOnATerminalRunWaitsOnNothing keeps a finished run from reading as
// stalled: nothing will arrive, so nothing is being waited on — while the
// requests left open stay listed, because they are what the end of the run
// interrupted.
func TestStallViewOnATerminalRunWaitsOnNothing(t *testing.T) {
	events := []Event{toolRequested("call-1", "read_file", "ses-parent", 1, 10)}
	for _, status := range []string{StatusCompleted, StatusFailed, StatusCancelled, StatusUnknown} {
		view := ObserveStall(stalledRun(status), events, nil)
		if view.Waiting != WaitNone {
			t.Fatalf("status %q: waiting = %q, want %q", status, view.Waiting, WaitNone)
		}
		if len(view.Pending) != 1 {
			t.Fatalf("status %q: the request left open must stay visible: %#v", status, view.Pending)
		}
	}
}

// TestStallViewDeclaresATruncatedWindow proves the caveat is observable instead
// of implicit. A window that starts above sequence 1 cannot prove a request was
// never answered, so the consumer is told the picture is partial.
func TestStallViewDeclaresATruncatedWindow(t *testing.T) {
	full := ObserveStall(stalledRun(StatusRunning), []Event{toolRequested("call-1", "read_file", "ses-parent", 1, 10)}, nil)
	if full.WindowTruncated {
		t.Fatal("a window starting at sequence 1 is not truncated")
	}
	truncated := ObserveStall(stalledRun(StatusRunning), []Event{toolRequested("call-9", "read_file", "ses-parent", 4000, 10)}, nil)
	if !truncated.WindowTruncated {
		t.Fatal("a window starting above sequence 1 must be reported as truncated")
	}
}

// TestStallViewOmitsRequestsItCannotPair keeps the view from accusing anything
// on the strength of a fact it does not have. A request with no identity can
// never be matched to its resolution, so listing it would report a permanent
// stall that Matrix cannot actually substantiate.
func TestStallViewOmitsRequestsItCannotPair(t *testing.T) {
	anonymous := toolRequested("", "read_file", "ses-parent", 1, 10)
	view := ObserveStall(stalledRun(StatusRunning), []Event{anonymous}, nil)
	if len(view.Pending) != 0 {
		t.Fatalf("an unpairable request was reported as pending: %#v", view.Pending)
	}
	if view.Waiting == WaitToolResult {
		t.Fatal("an unpairable request was reported as a pending tool wait Matrix cannot substantiate")
	}
}

// TestProjectionCarriesTheStallViewPastTheTracePolicy is the test that keeps the
// feature from working only in the laboratory configuration. Session attribution
// lives in protocol metadata and tool names are dropped in redacted mode, so a
// view computed after the trace policy goes blind on exactly the runs an operator
// diagnoses — the runs that do not opt into protocol metadata.
func TestProjectionCarriesTheStallViewPastTheTracePolicy(t *testing.T) {
	run := stalledRun(StatusRunning)
	run.TracePolicy = TracePolicy{ContentMode: ContentModeRefs, IncludeProtocolMeta: false}
	trace := Project(run, []Event{toolRequested("call-child", "grep", "ses-child", 7, 20)}, nil)
	if trace.Stall == nil {
		t.Fatal("the trace carries no stall view")
	}
	if len(trace.Stall.Sessions) != 2 || trace.Stall.Sessions[1].SessionID != "ses-child" {
		t.Fatalf("the stall view lost session attribution to the trace policy: %#v", trace.Stall.Sessions)
	}
	if trace.Events[0].ProtocolMeta != nil {
		t.Fatal("this test needs a policy that strips protocol metadata to prove anything")
	}
	if trace.Stall.Pending[0].Name != "grep" {
		t.Fatalf("the stall view lost the tool name: %#v", trace.Stall.Pending)
	}
}

// TestStallViewSurvivesATracePolicyThatRedactsToolNames covers the same failure
// in the harshest configuration: redacted traces erase tool names, but a stall
// view that cannot say which tool is stuck is not worth having.
func TestStallViewSurvivesATracePolicyThatRedactsToolNames(t *testing.T) {
	run := stalledRun(StatusRunning)
	run.TracePolicy = TracePolicy{ContentMode: ContentModeRedacted, IncludeProtocolMeta: false}
	trace := Project(run, []Event{toolRequested("call-1", "read_file", "ses-parent", 1, 10)}, nil)
	if trace.Stall == nil || len(trace.Stall.Pending) != 1 {
		t.Fatalf("stall view = %#v", trace.Stall)
	}
	if trace.Stall.Pending[0].Name != "read_file" {
		t.Fatalf("stall view lost the tool name to redaction: %#v", trace.Stall.Pending[0])
	}
}

// TestPermissionPairLeavesNoWaitWhenItClosesTogether pins the recorder contract
// the stall view depends on, so an empty permission wait is a documented fact
// rather than a silent gap. The notifier writes permission.requested and
// permission.resolved from the same update, after the policy decision is already
// made (internal/providers/agents/permission_events.go fills the decision before
// notifying), so a normal permission never appears as a wait. It appears only
// when the pair is broken, which is the case worth surfacing.
func TestPermissionPairLeavesNoWaitWhenItClosesTogether(t *testing.T) {
	events := []Event{
		toolRequested("call-1", "read_file", "ses-parent", 1, 10),
		permissionRequested("perm-1", "ses-parent", 2, 11),
		permissionResolved("perm-1", "ses-parent", 3, 11),
	}
	view := ObserveStall(stalledRun(StatusRunning), events, nil)
	if len(view.Pending) != 1 || view.Pending[0].ID != "call-1" {
		t.Fatalf("a decided permission must not stay pending: %#v", view.Pending)
	}
	if view.Waiting != WaitToolResult {
		t.Fatalf("waiting = %q, want the tool wait that is actually open", view.Waiting)
	}
	// The other half of the contract — a request whose resolution never got
	// written, which is an interrupted decision rather than a pending approval —
	// is covered by TestStallViewReportsAPendingPermission.
}

func elicitation(kind, id, session string, seconds int) Notification {
	return Notification{
		Kind: kind, RunID: "run-stall", ElicitationID: id,
		SessionID: session, Timestamp: at(seconds),
	}
}

// TestStallViewReportsTheApprovalTheRunIsBlockedOn covers the one wait a person
// holds up. The elicitation lifecycle is recorded as notifications, so the view
// reads that stream too; before it did, a run blocked on a human answer was
// indistinguishable from a run whose peer had gone quiet.
func TestStallViewReportsTheApprovalTheRunIsBlockedOn(t *testing.T) {
	opened := []Notification{elicitation(KindElicitationOpened, "elic-1", "ses-child", 30)}
	view := ObserveStall(stalledRun(StatusRunning), nil, opened)
	if view.Waiting != WaitElicitation {
		t.Fatalf("waiting = %q, want %q", view.Waiting, WaitElicitation)
	}
	if len(view.Pending) != 1 || view.Pending[0].ID != "elic-1" || view.Pending[0].Kind != WaitElicitation {
		t.Fatalf("pending = %#v, want the open elicitation", view.Pending)
	}
	if !view.WaitingSince.Equal(at(30)) {
		t.Fatalf("waiting since = %s, want %s", view.WaitingSince, at(30))
	}
	if len(view.Sessions) != 2 || view.Sessions[1].SessionID != "ses-child" {
		t.Fatalf("the elicitation was not attributed to the session that asked: %#v", view.Sessions)
	}

	closed := ObserveStall(stalledRun(StatusRunning), nil, append(opened, elicitation(KindElicitationResolved, "elic-1", "ses-child", 40)))
	if len(closed.Pending) != 0 {
		t.Fatalf("an answered elicitation is still pending: %#v", closed.Pending)
	}
	if closed.Waiting == WaitElicitation {
		t.Fatal("the run is still reported as blocked on an approval that already arrived")
	}
}

// TestStallViewOnlyTreatsElicitationOpeningsAsRequests keeps the view from
// reading the whole notification stream as requests. The stream also carries
// terminal wakeups, which restate what the events already say, and it will carry
// lifecycle kinds this view has never heard of. The second case is the one that
// needs a test: a future kind that carries an elicitation identity is exactly
// what a careless match would turn into a permanent pending approval.
func TestStallViewOnlyTreatsElicitationOpeningsAsRequests(t *testing.T) {
	notifications := []Notification{
		{Kind: "run.completed", RunID: "run-stall", Timestamp: at(5)},
		{Kind: "elicitation.expired", RunID: "run-stall", ElicitationID: "elic-1", SessionID: "ses-parent", Timestamp: at(6)},
	}
	view := ObserveStall(stalledRun(StatusRunning), nil, notifications)
	if len(view.Pending) != 0 {
		t.Fatalf("a notification that is not an opening became a pending request: %#v", view.Pending)
	}
	if view.Waiting != WaitUnknown {
		t.Fatalf("waiting = %q, want %q", view.Waiting, WaitUnknown)
	}
}

// TestProjectionCarriesTheHumanApprovalWait proves the wait survives all the way
// into the exported trace, which is what the consumer reads.
func TestProjectionCarriesTheHumanApprovalWait(t *testing.T) {
	trace := Project(stalledRun(StatusRunning), nil, []Notification{elicitation(KindElicitationOpened, "elic-1", "ses-parent", 30)})
	if trace.Stall == nil || trace.Stall.Waiting != WaitElicitation {
		t.Fatalf("stall = %#v, want the approval wait", trace.Stall)
	}
}
