package onboarding

import (
	"context"
	"fmt"
)

func (w *Wizard) handleSelectedAuthMethod(ctx context.Context, handler AuthHandler, method AuthMethod, state *WizardState) (string, error) {
	if state.AgentName == agentOpencode && method.ID == "quick_login" {
		return w.startOpenRouterOAuth(state)
	}
	if state.AgentName == agentCodex && method.ID == "chatgpt" {
		return w.startCodexDeviceAuth(ctx, handler, method, state)
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

func (w *Wizard) startCodexDeviceAuth(ctx context.Context, handler AuthHandler, method AuthMethod, state *WizardState) (string, error) {
	_, prompt, err := handler.Authenticate(ctx, method, "")
	if err != nil {
		state.Step = 3
		return fmt.Sprintf("⚠️ Could not start Codex login: %v", err), nil
	}
	state.Step = 4
	return prompt, nil
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

func (w *Wizard) handleOpencodeStep4(state *WizardState, input string) (string, bool, error) {
	if state.AgentName != agentOpencode || state.Context["auth_method"] != "" {
		return "", false, nil
	}
	if state.Context["provider"] == "OpenRouter" {
		response, err := w.handleOpenRouterAuthSelection(state, input)
		return response, true, err
	}
	if state.Context["provider"] != "" {
		response, err := w.handleOpencodeAPIKey(state, input)
		return response, true, err
	}
	return "", false, nil
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
