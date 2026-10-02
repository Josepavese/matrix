package onboarding

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// refusingAuthHandler advertises the methods a test hands it and always fails to
// start them, which is enough to tell a starter and the generic flow apart by
// their response and by the step they leave behind.
type refusingAuthHandler struct {
	methods []AuthMethod
}

func (h *refusingAuthHandler) Methods(context.Context) ([]AuthMethod, error) { return h.methods, nil }

func (h *refusingAuthHandler) Authenticate(context.Context, AuthMethod, string) (*AuthResult, string, error) {
	return nil, "", errors.New("cannot start")
}

func (h *refusingAuthHandler) Name() string { return "refusing" }

// TestVendorStarterStaysUnreachableForAReusedMethodID documents why a method id
// alone is not enough to reach a vendor starter.
//
// The generic ACP handler republishes whatever method ids an agent advertises in
// its initialize response, so an unrelated agent can advertise "chatgpt" or
// "quick_login". What keeps the vendor's device-auth or OAuth starter from
// running for it is that those starters belong to the handler that implements
// the flow — the fallback handler owns nothing — so a reused id cannot reach
// them even though the name is never consulted.
func TestVendorStarterStaysUnreachableForAReusedMethodID(t *testing.T) {
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

// TestCodexDeviceStarterRunsForItsOwnHandler is the positive half: the same
// method, held by the handler that implements the flow, does take the codex
// path — so the property above is not simply dead code.
//
// The handler is the real one, so the marker is produced by the real starter;
// only the process it would launch is refused, which is what makes the run of
// the starter visible without a codex binary on the machine.
func TestCodexDeviceStarterRunsForItsOwnHandler(t *testing.T) {
	w := newTestWizard()
	w.proc = stubProcess{}
	handler := &codexAuthHandler{wizard: w}
	method := AuthMethod{ID: "chatgpt", Name: "ChatGPT Login", Type: "agent"}
	state := &WizardState{
		Step:      4,
		Language:  "en",
		AgentName: agentCodex,
		Context:   map[string]string{"channel_id": "ch"},
	}

	resp, err := w.handleSelectedAuthMethod(context.Background(), handler, method, state)
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

// TestDeclaredFlowsBelongToTheirHandlers is the ownership vocabulary itself: a
// handler owns the method it starts and leaves every other method generic, which
// is what replaces the wizard asking who the agent is.
func TestDeclaredFlowsBelongToTheirHandlers(t *testing.T) {
	w := newTestWizard()
	// The device-auth starter launches the vendor binary; refusing the launch is
	// how the ownership answer is observed without that binary on the machine.
	w.proc = stubProcess{}
	state := &WizardState{Step: 3, Language: "en", Context: map[string]string{"channel_id": "ch"}}
	codex := &codexAuthHandler{wizard: w}
	openrouter := &openrouterAuthHandler{wizard: w}

	for _, test := range []struct {
		name     string
		handler  AuthHandler
		methodID string
		owned    bool
	}{
		{name: "codex device auth", handler: codex, methodID: "chatgpt", owned: true},
		{name: "codex api key", handler: codex, methodID: "openai-api-key", owned: false},
		{name: "openrouter oauth", handler: openrouter, methodID: "quick_login", owned: true},
		{name: "openrouter api key", handler: openrouter, methodID: "api_key", owned: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, owned, err := startDeclaredMethod(context.Background(), test.handler, AuthMethod{ID: test.methodID}, state)
			if err != nil {
				t.Fatalf("startDeclaredMethod: %v", err)
			}
			if owned != test.owned {
				t.Fatalf("method %q owned = %v, want %v", test.methodID, owned, test.owned)
			}
		})
	}
}
