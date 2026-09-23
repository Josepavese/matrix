package onboarding

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// fakeAuthControl stands in for the protocol side: it advertises what an agent
// would advertise and records what the wizard asks it to execute.
type fakeAuthControl struct {
	methods    []middleware.AuthenticationMethod
	methodsErr error
	authErr    error

	askedFor      []string
	authenticated []string
}

func (f *fakeAuthControl) AgentAuthenticationMethods(_ context.Context, agentID string) ([]middleware.AuthenticationMethod, error) {
	f.askedFor = append(f.askedFor, agentID)
	if f.methodsErr != nil {
		return nil, f.methodsErr
	}
	return f.methods, nil
}

func (f *fakeAuthControl) AuthenticateAgent(_ context.Context, agentID, methodID string) error {
	f.authenticated = append(f.authenticated, agentID+"/"+methodID)
	return f.authErr
}

// TestGenericHandlerOffersTheAgentsOwnMethods is the protocol-parity test the
// issue asked for: the method offered comes from the agent's initialize response,
// not from the hardcoded fallback. A test that supplies an AuthMethod by hand
// cannot catch this, because it provides the very input the real path failed to
// read.
func TestGenericHandlerOffersTheAgentsOwnMethods(t *testing.T) {
	control := &fakeAuthControl{methods: []middleware.AuthenticationMethod{{
		ID:          "opencode-login",
		Name:        "Login with opencode",
		Description: "Run `opencode auth login` in the terminal",
	}}}
	wizard := newTestWizard()
	wizard.SetAgentAuthController(control)

	handler := newAuthHandlerRegistry(wizard).get("crow-cli")

	methods, err := handler.Methods(context.Background())
	if err != nil {
		t.Fatalf("Methods: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("len(methods) = %d, want the one method the agent advertises: %+v", len(methods), methods)
	}
	if methods[0].ID != "opencode-login" {
		t.Fatalf("method = %q, want the advertised %q (a hardcoded method is the defect)", methods[0].ID, "opencode-login")
	}
	if methods[0].Type != "agent" {
		t.Fatalf("Type = %q, want %q for a stable ACP method", methods[0].Type, "agent")
	}
	if len(control.askedFor) != 1 || control.askedFor[0] != "crow-cli" {
		t.Fatalf("the agent was asked about the wrong id: %v", control.askedFor)
	}
}

// TestGenericHandlerExecutesTheAdvertisedMethod pins the other half: once the
// local step is done, MATRIX performs the protocol authenticate for the method
// the agent published. Reporting success without it is what made the capability
// decorative.
func TestGenericHandlerExecutesTheAdvertisedMethod(t *testing.T) {
	control := &fakeAuthControl{methods: []middleware.AuthenticationMethod{{
		ID:   "opencode-login",
		Name: "Login with opencode",
	}}}
	wizard := newTestWizard()
	wizard.SetAgentAuthController(control)
	handler := newAuthHandlerRegistry(wizard).get("crow-cli")

	methods, err := handler.Methods(context.Background())
	if err != nil {
		t.Fatalf("Methods: %v", err)
	}
	method := methods[0]

	// First call only shows the instruction.
	if _, prompt, err := handler.Authenticate(context.Background(), method, ""); err != nil || prompt == "" {
		t.Fatalf("first call must prompt: prompt=%q err=%v", prompt, err)
	}
	if len(control.authenticated) != 0 {
		t.Fatalf("nothing must be authenticated before the user is done: %v", control.authenticated)
	}

	// The user reports the terminal login is complete.
	result, prompt, err := handler.Authenticate(context.Background(), method, "done")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if prompt != "" {
		t.Fatalf("unexpected prompt %q", prompt)
	}
	if result == nil {
		t.Fatal("a completed flow must return a result")
	}
	if len(control.authenticated) != 1 || control.authenticated[0] != "crow-cli/opencode-login" {
		t.Fatalf("the protocol authenticate was not performed for the advertised method: %v", control.authenticated)
	}
}

// TestGenericHandlerReportsProtocolFailure keeps a rejected authenticate from
// looking like a completed login.
func TestGenericHandlerReportsProtocolFailure(t *testing.T) {
	control := &fakeAuthControl{
		methods: []middleware.AuthenticationMethod{{ID: "opencode-login", Name: "Login"}},
		authErr: errors.New("agent refused"),
	}
	wizard := newTestWizard()
	wizard.SetAgentAuthController(control)
	handler := newAuthHandlerRegistry(wizard).get("crow-cli")

	methods, _ := handler.Methods(context.Background())
	_, _, err := handler.Authenticate(context.Background(), methods[0], "done")
	if err == nil {
		t.Fatal("a rejected authenticate must surface as an error, not as success")
	}
	if !strings.Contains(err.Error(), "opencode-login") {
		t.Fatalf("the error must name the method: %v", err)
	}
}

// TestGenericHandlerFallsBackWhenTheAgentPublishesNothing keeps the previous
// behaviour for agents that advertise no method, and for the case where the
// protocol cannot be reached at all.
func TestGenericHandlerFallsBackWhenTheAgentPublishesNothing(t *testing.T) {
	for name, control := range map[string]*fakeAuthControl{
		"no methods":  {},
		"unreachable": {methodsErr: errors.New("agent did not start")},
	} {
		t.Run(name, func(t *testing.T) {
			wizard := newTestWizard()
			wizard.SetAgentAuthController(control)
			handler := newAuthHandlerRegistry(wizard).get("mystery-agent")

			methods, err := handler.Methods(context.Background())
			if err != nil {
				t.Fatalf("Methods: %v", err)
			}
			if len(methods) != 1 || methods[0].ID != "api_key" {
				t.Fatalf("methods = %+v, want the generic api_key fallback", methods)
			}

			// The fallback is not the agent's own method, so completing it must
			// not claim a protocol authorization that never happened.
			if _, _, err := handler.Authenticate(context.Background(), methods[0], "sk-test"); err != nil {
				t.Fatalf("Authenticate: %v", err)
			}
			if len(control.authenticated) != 0 {
				t.Fatalf("the fallback must not call the protocol: %v", control.authenticated)
			}
		})
	}
}

// TestGenericHandlerWithoutAControllerStillWorks covers the window before the
// router is wired: the handler must answer, not panic and not block.
func TestGenericHandlerWithoutAControllerStillWorks(t *testing.T) {
	wizard := newTestWizard()
	handler := newAuthHandlerRegistry(wizard).get("mystery-agent")

	methods, err := handler.Methods(context.Background())
	if err != nil {
		t.Fatalf("Methods: %v", err)
	}
	if len(methods) != 1 || methods[0].ID != "api_key" {
		t.Fatalf("methods = %+v, want the generic fallback", methods)
	}
}

// TestAgentAuthenticationControllerHasAConsumer is the "no dead port" assertion:
// the adapter exposes ACP authenticate through middleware's
// AgentAuthenticationController, and the generic handler is what consumes it. If
// the consumer disappears, this fails instead of leaving a documented capability
// nobody can invoke.
func TestAgentAuthenticationControllerHasAConsumer(t *testing.T) {
	control := &fakeAuthControl{methods: []middleware.AuthenticationMethod{{ID: "agent-method", Name: "Agent method"}}}
	wizard := newTestWizard()
	var consumer AgentAuthController = control
	wizard.SetAgentAuthController(consumer)
	if wizard.agentAuthController() == nil {
		t.Fatal("the wizard must expose the wired controller")
	}

	handler := newAuthHandlerRegistry(wizard).get("generic")
	methods, err := handler.Methods(context.Background())
	if err != nil {
		t.Fatalf("Methods: %v", err)
	}
	if len(control.askedFor) == 0 {
		t.Fatal("the controller was never asked for methods: the port has no consumer")
	}
	if methods[0].ID != "agent-method" {
		t.Fatalf("methods = %+v", methods)
	}
}
