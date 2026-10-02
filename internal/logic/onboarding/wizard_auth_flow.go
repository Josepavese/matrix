package onboarding

import (
	"context"
	"fmt"
)

func (w *Wizard) handleSelectedAuthMethod(ctx context.Context, handler AuthHandler, method AuthMethod, state *WizardState) (string, error) {
	if message, owned, err := startDeclaredMethod(ctx, handler, method, state); owned {
		return message, err
	}
	if method.Type == "env_var" {
		return w.promptForEnvAuth(ctx, handler, method, state), nil
	}
	return w.promptOrFinishAuth(ctx, handler, method, state)
}

func selectedMethodFromInput(input string, methods []AuthMethod) (AuthMethod, bool) {
	idx := parseSelectionIndex(input)
	if idx < 1 || idx > len(methods) {
		return AuthMethod{}, false
	}
	return methods[idx-1], true
}

func (w *Wizard) promptForEnvAuth(ctx context.Context, handler AuthHandler, method AuthMethod, state *WizardState) string {
	state.Step = 4
	_, prompt, _ := handler.Authenticate(ctx, method, "") // error handled: empty prompt is used as fallback
	if prompt != "" {
		return prompt
	}
	return w.promptForStep(*state)
}

func (w *Wizard) promptOrFinishAuth(ctx context.Context, handler AuthHandler, method AuthMethod, state *WizardState) (string, error) {
	_, prompt, err := handler.Authenticate(ctx, method, "")
	if err != nil {
		return fmt.Sprintf("⚠️ Error: %v", err), nil
	}
	if prompt != "" {
		state.Step = 4
		return prompt, nil
	}
	return w.finishConfiguration(*state)
}

func (w *Wizard) selectedAuthMethod(ctx context.Context, handler AuthHandler, state *WizardState) (AuthMethod, error) {
	methods, err := handler.Methods(ctx)
	if err != nil {
		return AuthMethod{}, err
	}
	methodID := state.Context["auth_method"]
	for _, method := range methods {
		if method.ID == methodID {
			return method, nil
		}
	}
	if len(methods) > 0 {
		return methods[0], nil
	}
	return AuthMethod{}, nil
}
