package runtrace

import "testing"

func TestToolIDsArePairedWithinTheirOwnRemoteSession(t *testing.T) {
	events := []Event{
		toolRequested("same-id", "parent-tool", "parent", 2, 10),
		toolRequested("same-id", "child-tool", "child", 3, 11),
		toolRequested("same-id", "parent-tool", "parent", 4, 12),
		toolReceived("same-id", "child-tool", "child", 5, 13),
	}
	view := ObserveStall(stalledRun(StatusRunning), events, nil)
	if len(view.Pending) != 1 || view.Pending[0].SessionID != "parent" || !view.Pending[0].Since.Equal(at(10)) {
		t.Fatalf("a child resolution cleared its parent or repeated upserts fabricated waits: %+v", view.Pending)
	}
	if view.Cause != WaitUnknown || view.PendingComplete || !view.WindowTruncated {
		t.Fatalf("a retained unresolved request claimed a current provider diagnosis: %+v", view)
	}
}

func TestResolvedToolInRetainedWindowIsNeverReportedPending(t *testing.T) {
	events := []Event{toolRequested("id", "shell", "s", 900, 10), toolReceived("id", "shell", "s", 901, 11)}
	view := ObserveStall(stalledRun(StatusRunning), events, nil)
	if len(view.Pending) != 0 || view.Waiting == WaitToolResult || view.Cause != WaitUnknown {
		t.Fatalf("completed tool or silence was turned into a current pending diagnosis: %+v", view)
	}
}
