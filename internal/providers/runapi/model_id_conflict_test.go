package runapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/middleware"
)

// TestModelIDConflictNamesTheAgentAndTheRemedy is the criterion the PM set for
// this 409: it must name the agent it refused and the action that resolves it,
// not state a rule. It goes through the real route, because a message nobody
// sends is not a message.
func TestModelIDConflictNamesTheAgentAndTheRemedy(t *testing.T) {
	mux := http.NewServeMux()
	server := NewServer(&runTestRouter{}).WithTraceStorage(memstore.New()).WithEndpointResolver(
		launchPolicyEndpointResolver{endpoint: middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindA2A}},
	)
	server.RegisterRoutes(mux)

	body := `{"channel_id":"ch","input":"do it","agent_id":"halfpocket","model_id":"minimax-m3"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, RunPathV1, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusConflict, w.Body.String())
	}
	message := w.Body.String()
	if !strings.Contains(message, "halfpocket") {
		t.Fatalf("the refusal does not name the agent it refused: %s", message)
	}
	// The two remedies the runtime status offers for the same window. If this
	// text changes, agentmgr's pending_apply has to change with it.
	for _, remedy := range []string{"restart the daemon", "wait for the next refresh"} {
		if !strings.Contains(message, remedy) {
			t.Fatalf("the refusal does not say what to do (%q): %s", remedy, message)
		}
	}
	if strings.Contains(message, "model_id is supported only for ACP agents") {
		t.Fatalf("the refusal is still the generic rule: %s", message)
	}
}

// TestModelIDConflictKeepsTheRemediesTheRuntimeStatusOffers pins the wording
// this 409 shares with agentmgr's pending_apply warning for the same window: an
// agent that is registered and not yet applied. Two surfaces describing one
// situation must not send the operator in two directions, so the remedies are
// asserted here by name.
//
// LIMIT, deliberate and declared: this is a pinned copy, not a live comparison.
// agentmgr builds that warning inside an unexported function, and the exported
// BuildRuntimeReport needs a registry and process harness to reach that branch.
// Rewording agentmgr alone would not fail this test. The agreement was verified
// by reading both strings on 2026-10-01; whoever rewords one must reword the
// other, and the honest fix would be an exported accessor for the warning.
func TestModelIDConflictKeepsTheRemediesTheRuntimeStatusOffers(t *testing.T) {
	message := modelIDConflictMessage("halfpocket")
	for _, remedy := range []string{"restart the daemon", "wait for the next refresh"} {
		if !strings.Contains(message, remedy) {
			t.Fatalf("the 409 no longer offers the remedy the runtime status offers (%q): %s", remedy, message)
		}
	}
}

// TestModelIDConflictWithoutAResolvedAgent stays honest when no agent was
// resolved: it must not print an empty name as if one had been found.
func TestModelIDConflictWithoutAResolvedAgent(t *testing.T) {
	message := modelIDConflictMessage("   ")
	if strings.Contains(message, `""`) {
		t.Fatalf("the refusal prints an empty agent name: %s", message)
	}
	if !strings.Contains(message, "the requested agent") {
		t.Fatalf("the refusal does not say which agent it means: %s", message)
	}
}
