package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/runapi"
)

// stubEndpointResolver answers the protocol family a test needs, so the real
// refusal path under test is reached without a running supervisor.
type stubEndpointResolver struct {
	endpoint middleware.ProtocolEndpoint
}

func (s stubEndpointResolver) GetAgentEndpoint(string) (middleware.ProtocolEndpoint, error) {
	return s.endpoint, nil
}

// TestRunAPIRefusalSharesTheRegistrationRemedyWithTheRuntimeStatus is the live
// comparison between the two surfaces an operator meets in the same window: the
// run API refuses a request that depends on a registration the runtime has not
// applied, and the runtime status reports that same registration as
// pending_apply.
//
// The assertion is built from the shared source rather than from a copy of the
// sentence, so rewording the remedy on either side — here or in agentmgr —
// fails this test instead of leaving two surfaces telling the operator two
// different things to do.
func TestRunAPIRefusalSharesTheRegistrationRemedyWithTheRuntimeStatus(t *testing.T) {
	runtime := runapi.NewServer(nil).WithEndpointResolver(stubEndpointResolver{
		endpoint: middleware.ProtocolEndpoint{Kind: middleware.ProtocolKind("a2a")},
	})

	request := httptest.NewRequest(http.MethodPost, runapi.RunPathV1,
		strings.NewReader(`{"channel_id":"ch-1","input":"ciao","agent_id":"mimo","model_id":"gpt-5"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	runtime.HandleRuns(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %q; want 409 for a model_id on a non-ACP agent", recorder.Code, recorder.Body.String())
	}
	message := recorder.Body.String()
	if !strings.Contains(message, middleware.RegistrationPendingRemedy) {
		t.Fatalf("the refusal does not carry the shared remedy %q: %q", middleware.RegistrationPendingRemedy, message)
	}
	if !strings.Contains(message, middleware.RegistrationRemedyThen("retry")) {
		t.Fatalf("the refusal does not carry the shared remedy with its own next step: %q", message)
	}
	if !strings.Contains(message, "mimo") {
		t.Fatalf("the refusal must name the agent it was decided for: %q", message)
	}
}
