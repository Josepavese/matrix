package onboarding

import "context"

// Flow ownership: the wizard resolves ONE AuthHandler for the selected agent —
// that is the registry's job — and then asks that handler what it owns, instead
// of comparing the agent's name against a list of the agents whose flows the
// wizard happens to know.
//
// Why ownership rather than the method id alone: the generic ACP handler
// republishes whichever method ids an agent advertises in its initialize
// response, so a method id on its own is not a claim about the flow behind it —
// an unrelated agent can advertise "chatgpt". Ownership is a property of the
// handler that implements the flow, so an agent whose handler declares nothing
// keeps the generic path even when it reuses the id, and a new agent served by
// an existing handler is onboarded without touching the wizard.
//
// A handler that implements none of the interfaces below owns nothing: every
// step stays generic. That is the case for the fallback handler, which answers
// for every agent the registry does not know.

// selectionPreparer is implemented by a handler whose provider needs work after
// it was selected and before the auth step: installing its own binary, or
// detecting that it is already authenticated and finishing the flow.
type selectionPreparer interface {
	PrepareSelection(state *WizardState) (string, error)
}

// methodStarter is implemented by a handler that starts one of the auth methods
// it declares with its own flow instead of the generic authenticate call. It
// reports owned=false for a method it does not start.
type methodStarter interface {
	StartDeclaredMethod(ctx context.Context, method AuthMethod, state *WizardState) (string, bool, error)
}

// providerAuthOwner is implemented by a handler whose flow asks the operator for
// a provider and then for the credential of that provider, between the method
// and the model selection.
type providerAuthOwner interface {
	// ProviderAuthPrompt is the prompt of the stage it owns; an empty prompt
	// leaves the stage to the generic text.
	ProviderAuthPrompt(state *WizardState) string
	// HandleProviderAuthInput consumes the input of the stage it owns, or reports
	// owned=false.
	HandleProviderAuthInput(state *WizardState, input string) (string, bool, error)
}

// prepareSelection asks the handler resolved for the selected agent to do its own
// pre-auth work. owned=false means the flow has none and the wizard continues on
// the generic path.
func prepareSelection(handler AuthHandler, state *WizardState) (string, bool, error) {
	preparer, ok := handler.(selectionPreparer)
	if !ok {
		return "", false, nil
	}
	message, err := preparer.PrepareSelection(state)
	return message, true, err
}

// startDeclaredMethod routes a chosen method through the handler's own starter,
// or reports that the generic path keeps it.
func startDeclaredMethod(ctx context.Context, handler AuthHandler, method AuthMethod, state *WizardState) (string, bool, error) {
	starter, ok := handler.(methodStarter)
	if !ok {
		return "", false, nil
	}
	return starter.StartDeclaredMethod(ctx, method, state)
}

// providerAuthPrompt is the prompt of the provider-auth stage, or "" when the
// resolved handler does not own it.
func providerAuthPrompt(handler AuthHandler, state *WizardState) string {
	owner, ok := handler.(providerAuthOwner)
	if !ok {
		return ""
	}
	return owner.ProviderAuthPrompt(state)
}

// handleProviderAuthInput gives the stage's input to the handler that owns it.
func handleProviderAuthInput(handler AuthHandler, state *WizardState, input string) (string, bool, error) {
	owner, ok := handler.(providerAuthOwner)
	if !ok {
		return "", false, nil
	}
	return owner.HandleProviderAuthInput(state, input)
}
