package onboarding

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// errNoProcess stands for a host where the vendor binary cannot be launched;
// it is how a test observes that the starter ran without needing the binary.
var errNoProcess = errors.New("no process in this test")

// stubProcess is a Process whose every operation is refused. hasExecutable
// decides what HasExecutable answers, so a handler that installs its own binary
// can be walked past its installation step without an installer.
type stubProcess struct {
	hasExecutable bool
}

func (stubProcess) Exec(middleware.CommandSpec) ([]byte, error) { return nil, errNoProcess }

func (stubProcess) ExecSeparate(context.Context, middleware.CommandSpec) (*middleware.ExecResult, error) {
	return nil, errNoProcess
}

func (stubProcess) Start(middleware.CommandSpec) (middleware.ProcessHandle, error) {
	return nil, errNoProcess
}

func (stubProcess) StartPiped(middleware.CommandSpec) (middleware.PipedProcess, error) {
	return nil, errNoProcess
}

func (stubProcess) RunPrivileged(middleware.CommandSpec) ([]byte, error) { return nil, errNoProcess }

func (p stubProcess) HasExecutable(string) bool { return p.hasExecutable }

func (stubProcess) SpawnPTY() error { return errNoProcess }

// TestFallbackHandlerOwnsNoFlow pins the property that makes the vendor starters
// unreachable for every agent the registry does not know: the fallback handler
// declares none of the ownership capabilities, so a brand-new agent keeps the
// generic path without anything in the wizard naming it.
func TestFallbackHandlerOwnsNoFlow(t *testing.T) {
	w := newTestWizard()
	fallback := w.handlers.get("agent-nobody-has-seen")
	state := &WizardState{Step: 4, Language: "en", Context: map[string]string{"channel_id": "ch", "provider": "OpenRouter"}}

	if message, owned, err := prepareSelection(fallback, state); owned || err != nil || message != "" {
		t.Fatalf("the fallback handler claimed selection preparation: owned=%v err=%v message=%q", owned, err, message)
	}
	if _, owned, err := startDeclaredMethod(context.Background(), fallback, AuthMethod{ID: "chatgpt"}, state); owned || err != nil {
		t.Fatalf("the fallback handler claimed a declared method: owned=%v err=%v", owned, err)
	}
	if prompt := providerAuthPrompt(fallback, state); prompt != "" {
		t.Fatalf("the fallback handler produced a provider-auth prompt: %q", prompt)
	}
	if _, handled, err := handleProviderAuthInput(fallback, state, "1"); handled || err != nil {
		t.Fatalf("the fallback handler consumed a provider-auth input: handled=%v err=%v", handled, err)
	}
}

// TestProviderAuthStageStaysGenericWithoutAnOwner is the negative half of the
// provider-auth stage: only the handler serving the provider produces its prompt
// and consumes its input; any other handler leaves the stage to the wizard.
func TestProviderAuthStageStaysGenericWithoutAnOwner(t *testing.T) {
	w := newTestWizard()
	openrouter := &openrouterAuthHandler{wizard: w}
	state := &WizardState{
		Step:      4,
		Language:  "en",
		AgentName: "opencode",
		Context:   map[string]string{"channel_id": "ch", "provider": "OpenRouter"},
	}

	prompt := providerAuthPrompt(openrouter, state)
	if !strings.Contains(prompt, "OpenRouter auth") {
		t.Fatalf("the owning handler did not produce its provider-auth prompt: %q", prompt)
	}

	// A handler that does not serve this stage must not speak for it, and must
	// not consume the input: the wizard then takes the generic path, which for an
	// agent with no auth methods reports the error and leaves the state alone.
	unowned := &refusingAuthHandler{}
	if prompt := providerAuthPrompt(unowned, state); prompt != "" {
		t.Fatalf("a handler without the provider-auth stage produced a prompt: %q", prompt)
	}
	if _, handled, err := handleProviderAuthInput(unowned, state, "1"); handled || err != nil {
		t.Fatalf("a handler without the provider-auth stage consumed the input: handled=%v err=%v", handled, err)
	}

	// The same stage through the step entry point: the wizard resolves the
	// handler for the agent in state, so an agent served by another handler
	// cannot land in the provider-auth flow.
	foreign := &WizardState{
		Step:      4,
		Language:  "en",
		AgentName: "mimo",
		Context:   map[string]string{"channel_id": "ch", "provider": "OpenRouter"},
	}
	resp, err := w.step4Handle(nil, foreign, "1")
	if err != nil {
		t.Fatalf("step4Handle: %v", err)
	}
	if strings.Contains(resp, "OpenRouter auth") || foreign.Context["api_key"] != "" {
		t.Fatalf("an agent without the provider-auth flow was routed into it: %q (api_key=%q)", resp, foreign.Context["api_key"])
	}
}

// TestSelectionPreparationBelongsToTheCodexHandler pins the pre-auth step: the
// codex handler prepares its own selection and every other handler declines, so
// an agent that is not codex cannot be sent through the codex installation.
func TestSelectionPreparationBelongsToTheCodexHandler(t *testing.T) {
	w := newTestWizard()
	// The binary is present, so the handler walks past its installation step
	// instead of asking a nil installer to install anything.
	w.proc = stubProcess{hasExecutable: true}
	state := &WizardState{Step: 3, Language: "en", AgentName: agentCodex, Context: map[string]string{"channel_id": "ch"}}

	message, owned, err := prepareSelection(&codexAuthHandler{wizard: w}, state)
	if err != nil {
		t.Fatalf("prepareSelection: %v", err)
	}
	if !owned {
		t.Fatal("the codex handler declined its own selection preparation")
	}
	if message == "" {
		t.Fatal("the codex handler prepared nothing and said nothing")
	}

	if message, owned, err := prepareSelection(&refusingAuthHandler{}, state); owned || err != nil || message != "" {
		t.Fatalf("a foreign handler claimed the codex preparation: owned=%v err=%v message=%q", owned, err, message)
	}
}
