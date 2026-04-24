package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	agentCodex    = "codex"
	agentOpencode = "opencode"
)

func (w *Wizard) getStepRegistry() map[int]WizardStep {
	return map[int]WizardStep{
		1: {Prompt: w.step1Prompt, Handle: w.step1Handle},
		2: {Prompt: w.step2Prompt, Handle: w.step2Handle},
		3: {Prompt: w.step3Prompt, Handle: w.step3Handle},
		4: {Prompt: w.step4Prompt, Handle: w.step4Handle},
		5: {Prompt: w.step5Prompt, Handle: w.step5Handle},
	}
}

// step1: Language selection
func (w *Wizard) step1Prompt(_ *Wizard, _ *WizardState) string {
	return w.localizer.GetString("en", "language_selection")
}

func (w *Wizard) step1Handle(_ *Wizard, state *WizardState, input string) (string, error) {
	selectedLang, ok := map[string]string{"1": "en", "2": "it"}[input]
	if !ok {
		return w.invalidSelection(state, w.localizer.GetString("en", "language_selection")), nil
	}

	langData, err := json.Marshal(selectedLang)
	if err != nil {
		return "", err
	}
	if err := w.storage.Set("system.language", langData); err != nil {
		return "", err
	}
	if err := w.storage.Set("channel.language."+state.Context["channel_id"], langData); err != nil {
		return "", err
	}

	state.Language = selectedLang
	state.Step = 2
	return w.promptForStep(*state), nil
}

// step2: Agent selection from the configured discovery policy
func (w *Wizard) step2Prompt(_ *Wizard, state *WizardState) string {
	agents, err := w.fetchAgentList()
	if err != nil || len(agents) == 0 {
		return w.localizer.GetString(state.Language, "agent_selection_header") + "(no agents available)"
	}

	// Cache agent list in state for step2Handle
	agentData, err := json.Marshal(agents)
	if err != nil {
		return "Failed to serialize agent list."
	}
	state.Context["_agent_list"] = string(agentData)

	text := w.localizer.GetString(state.Language, "agent_selection_header")
	for i, a := range agents {
		mark := ""
		if a.Installed {
			mark = " " + w.localizer.GetString(state.Language, "agent_installed_mark")
		}
		distInfo := ""
		if len(a.DistTypes) > 0 {
			distInfo = fmt.Sprintf(" (%s)", strings.Join(a.DistTypes, ", "))
		}
		desc := ""
		if a.Description != "" {
			runes := []rune(a.Description)
			if len(runes) > 50 {
				desc = " — " + string(runes[:47]) + "..."
			} else {
				desc = " — " + a.Description
			}
		}
		text += fmt.Sprintf("[%d] %s%s%s%s\n", i+1, a.Name, distInfo, desc, mark)
	}
	return text
}

func (w *Wizard) step2Handle(_ *Wizard, state *WizardState, input string) (string, error) {
	agents, ok := w.cachedOrFetchedAgents(state)
	if !ok {
		return w.invalidSelection(state, w.promptForStep(*state)), nil
	}

	selected, ok := selectedAgentFromInput(input, agents)
	if !ok {
		return w.invalidSelection(state, w.promptForStep(*state)), nil
	}

	channelID := state.Context["channel_id"]
	state.AgentName = selected.ID
	state.Context = map[string]string{"channel_id": channelID}
	state.Step = 3

	if !selected.Installed && selected.Source != "" && w.activator != nil {
		return w.activateSelectedAgent(state, selected, channelID)
	}
	if selected.ID == agentCodex {
		return w.prepareCodexSelection(state)
	}
	return w.promptForStep(*state), nil
}

func (w *Wizard) prepareCodexSelection(state *WizardState) (string, error) {
	installMsg, err := w.ensureCodexInstalled()
	if err != nil {
		return fmt.Sprintf("⚠️ Could not install Codex: %v\n\nPlease install it manually: npm install -g @openai/codex", err), nil
	}

	handler := w.handlers.get(agentCodex)
	codexHandler, ok := handler.(*codexAuthHandler)
	if !ok || !codexHandler.isCodexAuthenticated() {
		return installMsg + w.promptForStep(*state), nil
	}

	result, err := w.finishConfiguration(*state)
	return installMsg + "✅ Codex is already authenticated.\n" + result, err
}

// step3: Auth method selection — uses AuthHandler.Methods() for dynamic dispatch
func (w *Wizard) step3Prompt(_ *Wizard, state *WizardState) string {
	handler := w.handlers.get(state.AgentName)
	ctx := context.Background()
	methods, err := handler.Methods(ctx)
	if err != nil || len(methods) == 0 {
		// Fallback to generic API key prompt
		return fmt.Sprintf(w.localizer.GetString(state.Language, "generic_api_key_prompt"), state.AgentName)
	}

	if len(methods) == 1 {
		// Only one method: show its prompt directly
		// Store the method for step4 to use
		state.Context["auth_method"] = methods[0].ID
		// Generate the prompt via Authenticate with empty input
		_, prompt, err := handler.Authenticate(ctx, methods[0], "")
		if err == nil && prompt != "" {
			return prompt
		}
		return fmt.Sprintf(w.localizer.GetString(state.Language, "generic_api_key_prompt"), state.AgentName)
	}

	// Multiple methods: show selection menu
	text := fmt.Sprintf("How would you like to authenticate %s?\n\n", state.AgentName)
	for i, m := range methods {
		desc := ""
		if m.Description != "" {
			desc = fmt.Sprintf(" — %s", m.Description)
		}
		text += fmt.Sprintf("%d. %s%s\n", i+1, m.Name, desc)
	}
	return text
}

func (w *Wizard) step3Handle(_ *Wizard, state *WizardState, input string) (string, error) {
	handler := w.handlers.get(state.AgentName)
	ctx := context.Background()
	methods, err := handler.Methods(ctx)
	if err != nil || len(methods) == 0 {
		state.Context["api_key"] = input
		return w.finishConfiguration(*state)
	}

	if len(methods) == 1 {
		return w.handleAuthInput(handler, methods[0], state, input)
	}

	method, ok := selectedMethodFromInput(input, methods)
	if !ok {
		return w.invalidSelection(state, w.promptForStep(*state)), nil
	}

	state.Context["auth_method"] = method.ID
	return w.handleSelectedAuthMethod(ctx, handler, method, state)
}

// step4: Auth input / completion
func (w *Wizard) step4Prompt(_ *Wizard, state *WizardState) string {
	// For opencode provider selection flow
	if state.AgentName == agentOpencode && state.Context["provider"] == "OpenRouter" && state.Context["auth_method"] == "" {
		return w.localizer.GetString(state.Language, "opencode_auth_method_prompt")
	}
	// For opencode API key after provider selected
	if state.AgentName == agentOpencode && state.Context["provider"] != "" && state.Context["auth_method"] == "" {
		return fmt.Sprintf(w.localizer.GetString(state.Language, "opencode_api_key_prompt"), state.Context["provider"])
	}

	return "Reply 'done' when you have completed authentication."
}

func (w *Wizard) step4Handle(_ *Wizard, state *WizardState, input string) (string, error) {
	if response, handled, err := w.handleOpencodeStep4(state, input); handled {
		return response, err
	}

	handler := w.handlers.get(state.AgentName)
	ctx := context.Background()
	method, err := w.selectedAuthMethod(ctx, handler, state)
	if err != nil {
		return fmt.Sprintf("⚠️ Error loading auth methods: %v", err), nil
	}
	return w.handleAuthInput(handler, method, state, input)
}

// step5: Model selection (opencode only)
func (w *Wizard) step5Prompt(_ *Wizard, state *WizardState) string {
	return w.localizer.GetString(state.Language, "opencode_model_prompt")
}

func (w *Wizard) step5Handle(_ *Wizard, state *WizardState, input string) (string, error) {
	state.Context["model"] = input
	return w.finishConfiguration(*state)
}

func (w *Wizard) invalidSelection(state *WizardState, prompt string) string {
	return w.localizer.GetString(state.Language, "invalid_selection_number") + "\n\n" + prompt
}

func parseSelectionIndex(input string) int {
	idx := 0
	for _, c := range input {
		if c < '0' || c > '9' {
			break
		}
		idx = idx*10 + int(c-'0')
	}
	return idx
}

// handleAuthInput processes user input for a given auth method via the handler.
// On success, stores AuthResult.Env in state context and finishes configuration.
func (w *Wizard) handleAuthInput(handler AuthHandler, method AuthMethod, state *WizardState, input string) (string, error) {
	ctx := context.Background()
	result, prompt, err := handler.Authenticate(ctx, method, input)
	if err != nil {
		return fmt.Sprintf("⚠️ Authentication error: %v", err), nil
	}

	// Handler returned a prompt — show it and stay on current step
	if prompt != "" {
		return prompt, nil
	}

	// Handler returned a result — store credentials and finish
	if result != nil {
		if len(result.Env) > 0 {
			envJSON, err := json.Marshal(result.Env)
			if err != nil {
				return "", fmt.Errorf("failed to serialize auth credentials: %w", err)
			}
			state.Context["_auth_env"] = string(envJSON)
		}
		return w.finishConfiguration(*state)
	}

	// Neither prompt nor result — shouldn't happen but handle gracefully
	return w.finishConfiguration(*state)
}

// startOpenRouterOAuth initiates the OpenRouter OAuth PKCE flow for opencode.
func (w *Wizard) startOpenRouterOAuth(state *WizardState) (string, error) {
	handler := w.handlers.get(agentOpencode)
	orHandler, ok := handler.(*openrouterAuthHandler)
	if !ok {
		return "⚠️ OpenRouter auth not available", nil
	}

	url, verifier, err := orHandler.generateAuthURL(state.Context["channel_id"])
	if err != nil {
		return fmt.Sprintf("⚠️ Could not generate Auth URL: %v", err), nil
	}
	state.Context["pkce_verifier"] = verifier
	state.Context["auth_method"] = "quick_login"
	state.Step = 4
	return fmt.Sprintf(w.localizer.GetString(state.Language, "opencode_openrouter_quick_auth_prompt"), url), nil
}

// handleOpenRouterAuthSelection handles the OpenRouter auth method selection for opencode.
func (w *Wizard) handleOpenRouterAuthSelection(state *WizardState, input string) (string, error) {
	choice, ok := map[string]string{"1": "api_key", "2": "quick_login"}[input]
	if !ok {
		return w.invalidSelection(state, w.promptForStep(*state)), nil
	}

	state.Context["auth_method"] = choice
	if choice != "quick_login" {
		state.Step = 4
		return w.promptForStep(*state), nil
	}

	return w.startOpenRouterOAuth(state)
}

// handleOpencodeAPIKey handles API key input for opencode with non-OpenRouter providers.
func (w *Wizard) handleOpencodeAPIKey(state *WizardState, input string) (string, error) {
	if strings.EqualFold(input, "skip") {
		input = ""
	}
	state.Context["api_key"] = input
	state.Step = 5
	return w.promptForStep(*state), nil
}
