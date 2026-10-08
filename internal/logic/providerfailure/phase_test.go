package providerfailure

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

func TestAPIFailureAfterToolExecutionIsRuntimeAndNeverPreflight(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{AgentID: "agent", ChannelID: "work"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AppendEvent(runtrace.Event{RunID: run.ID, Kind: "tool.result.received", ToolCallID: "written"})
	if err != nil {
		t.Fatal(err)
	}
	cause := &codedRPCError{code: -32603, message: "Usage limit reached for 5 hour. Reset at 02:32:19", data: map[string]any{"errorName": "APIError"}}
	code, message := DefaultClassification("session/prompt", cause)
	failure := &Failure{Code: code, Message: message, Phase: "session/prompt", Diagnostics: Diagnostics(middleware.ProtocolEndpoint{}, cause), Err: cause}
	if _, err := store.Fail(run.ID, failure); err != nil {
		t.Fatal(err)
	}
	AppendRunEvent(store, run.ID, failure)
	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Kind == "provider.preflight.failed" {
			t.Fatal("work already executed but the failure says it never started")
		}
		if event.Kind == "provider.runtime.failed" {
			found = event.Metadata["code"] == APIError && event.Metadata["rpc_error_code"] == "-32603"
			if event.Metadata["retry_after"] != nil || event.Metadata["reset_at"] != nil {
				t.Fatal("quota prose was turned into invented structural scheduling data")
			}
		}
	}
	if !found {
		t.Fatal("the runtime API failure or its original RPC code was lost")
	}
	items, _, err := store.LoadNotificationsAfter(0, 100, nil)
	if err != nil || len(items) != 1 || items[0].FailureCode != APIError {
		t.Fatalf("terminal wakeup must report the failure once: %v %+v", err, items)
	}
}

func TestSetupErrorRemainsPreflight(t *testing.T) {
	cause := &codedRPCError{code: -32603, message: "refused"}
	code, message := DefaultClassification("session/new", cause)
	failure := &Failure{Code: code, Message: message, Phase: "session/new", Err: cause}
	if code != PreflightFailed || failure.EventKind() != "provider.preflight.failed" {
		t.Fatalf("setup classification changed: %+v", failure)
	}
}
