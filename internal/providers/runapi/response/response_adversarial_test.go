package response

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
)

// TestNewSuccessBuildsStableLinks pins the URLs the client follows; a run id with
// a trailing slash or a missing prefix would produce a link that 404s.
func TestNewSuccessBuildsStableLinks(t *testing.T) {
	success := Builder{Prefix: "/v1/runs/"}.NewSuccess("run-1", "completed", "hello")
	if success.RunID != "run-1" || success.Status != "completed" || success.Output != "hello" {
		t.Fatalf("unexpected success payload: %+v", success)
	}
	if success.TraceURL != "/v1/runs/run-1/trace" || success.EventsURL != "/v1/runs/run-1/events" || success.ActionsURL != "/v1/runs/run-1/actions" {
		t.Fatalf("unexpected links: %+v", success)
	}
	if success.Cleanup != nil {
		t.Fatal("no cleanup result was passed, so none must be attached")
	}
}

func TestNewErrorForErrorUsesTheProviderCodeOnlyForProviderFailures(t *testing.T) {
	builder := Builder{Prefix: "/v1/runs/"}

	typed := builder.NewErrorForError("run-2", "failed", &providerfailure.Failure{
		Code:        providerfailure.ModelUnavailable,
		Message:     "no such model",
		AgentID:     "codex",
		Diagnostics: map[string]string{"provider_exit_code": "1"},
	}, nil)
	if typed.Code != providerfailure.ModelUnavailable {
		t.Fatalf("a provider failure must surface its code, got %q", typed.Code)
	}
	if typed.Details["agent_id"] != "codex" || typed.Details["provider_exit_code"] != "1" {
		t.Fatalf("a provider failure must surface its details: %v", typed.Details)
	}
	if typed.Error == "" {
		t.Fatal("the error text must be preserved")
	}

	// A plain error must not be dressed up as a provider failure: a wrong code
	// would send the operator chasing the wrong subsystem.
	plain := builder.NewErrorForError("run-3", "failed", errors.New("disk full"), nil)
	if plain.Code != "" || plain.Details != nil {
		t.Fatalf("a plain error must not carry a provider code: %+v", plain)
	}
	if plain.Error != "disk full" {
		t.Fatalf("the error text was lost: %q", plain.Error)
	}

	// A nil error must not panic and must still describe the run.
	nilErr := builder.NewErrorForError("run-4", "failed", nil, nil)
	if nilErr.Error != "" || nilErr.RunID != "run-4" || nilErr.TraceURL != "/v1/runs/run-4/trace" {
		t.Fatalf("unexpected nil-error payload: %+v", nilErr)
	}
}

// TestCleanupIsAttachedWhenProvided covers both option-bearing paths, which are
// easy to drop silently when the call site is refactored.
func TestCleanupIsAttachedWhenProvided(t *testing.T) {
	cleanup := &middleware.SessionCleanupResult{Clean: true}
	success := Builder{Prefix: "/v1/runs/"}.NewSuccess("run-6", "completed", "ok", cleanup)
	if success.Cleanup != cleanup {
		t.Fatal("the success payload dropped the cleanup result")
	}
	failure := Builder{Prefix: "/v1/runs/"}.NewError("run-7", "failed", "boom", cleanup)
	if failure.Cleanup != cleanup {
		t.Fatal("the error payload dropped the cleanup result")
	}
}

// TestErrorPayloadOmitsEmptyFields keeps the wire contract free of empty codes
// and details: the client treats a present field as meaningful.
func TestErrorPayloadOmitsEmptyFields(t *testing.T) {
	encoded, err := json.Marshal(Builder{}.NewError("run-5", "failed", "boom", nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, absent := range []string{"code", "details", "cleanup"} {
		if _, ok := decoded[absent]; ok {
			t.Fatalf("field %q must be omitted when empty: %s", absent, encoded)
		}
	}
	for _, present := range []string{"run_id", "status", "error", "trace_url", "events_url", "actions_url"} {
		if _, ok := decoded[present]; !ok {
			t.Fatalf("field %q must always be present: %s", present, encoded)
		}
	}
}
