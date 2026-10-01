package onboarding

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// refusingAuthHandler advertises the methods a test hands it and always fails to
// start them, which is enough to tell the two starters and the generic flow
// apart by their response and by the step they leave behind.
type refusingAuthHandler struct {
	methods []AuthMethod
}

func (h *refusingAuthHandler) Methods(context.Context) ([]AuthMethod, error) { return h.methods, nil }

func (h *refusingAuthHandler) Authenticate(context.Context, AuthMethod, string) (*AuthResult, string, error) {
	return nil, "", errors.New("cannot start")
}

func (h *refusingAuthHandler) Name() string { return "refusing" }

// TestVendorStarterNeedsMoreThanAReusedMethodID documents why the two vendor
// starters in handleSelectedAuthMethod are guarded by the agent's identity.
//
// The generic ACP handler republishes whatever method ids an agent advertises in
// its initialize response, so a method id alone is not a claim about the flow
// behind it: an unrelated agent can advertise "chatgpt" or "quick_login". The
// guard is what keeps a vendor's device-auth or OAuth starter from running for an
// agent that merely reuses the id, so removing the identity half of these
// conditions is not a safe simplification.
func TestVendorStarterNeedsMoreThanAReusedMethodID(t *testing.T) {
	for _, test := range []struct {
		methodID string
		// marker is lower-case text only the vendor starter can produce.
		marker string
	}{
		{methodID: "chatgpt", marker: "Could not start Codex login"},
		{methodID: "quick_login", marker: "openrouter"},
	} {
		t.Run(test.methodID, func(t *testing.T) {
			w := newTestWizard()
			handler := &refusingAuthHandler{methods: []AuthMethod{{
				ID:   test.methodID,
				Name: "Reused id",
				Type: "agent",
			}}}
			method := handler.methods[0]

			// An unrelated agent that advertises the same id must stay on the
			// generic flow: no vendor starter, and the step is left untouched.
			// Starting from the method-selection step makes a starter that
			// advances the flow visible as well as a starter that speaks.
			state := &WizardState{
				Step:      3,
				Language:  "en",
				AgentName: "mimo",
				Context:   map[string]string{"channel_id": "ch"},
			}
			resp, err := w.handleSelectedAuthMethod(context.Background(), handler, method, state)
			if err != nil {
				t.Fatalf("handleSelectedAuthMethod: %v", err)
			}
			if strings.Contains(strings.ToLower(resp), strings.ToLower(test.marker)) {
				t.Fatalf("agent %q advertising method %q was routed into the vendor starter: %q",
					state.AgentName, test.methodID, resp)
			}
			if state.Step != 3 {
				t.Fatalf("agent %q advertising method %q had its step changed to %d by a vendor starter",
					state.AgentName, test.methodID, state.Step)
			}
		})
	}
}

// TestCodexDeviceStarterRunsForCodex is the positive half: the same method, on
// the agent the starter belongs to, does take the codex path — so the guard above
// is not simply dead code.
func TestCodexDeviceStarterRunsForCodex(t *testing.T) {
	w := newTestWizard()
	handler := &refusingAuthHandler{methods: []AuthMethod{{
		ID:   "chatgpt",
		Name: "ChatGPT Login",
		Type: "agent",
	}}}
	state := &WizardState{
		Step:      4,
		Language:  "en",
		AgentName: agentCodex,
		Context:   map[string]string{"channel_id": "ch"},
	}

	resp, err := w.handleSelectedAuthMethod(context.Background(), handler, handler.methods[0], state)
	if err != nil {
		t.Fatalf("handleSelectedAuthMethod: %v", err)
	}
	if !strings.Contains(resp, "Could not start Codex login") {
		t.Fatalf("the codex starter did not run for %s: %q", agentCodex, resp)
	}
	if state.Step != 3 {
		t.Fatalf("a failed device-auth start should send the user back to method selection, step = %d", state.Step)
	}
}
