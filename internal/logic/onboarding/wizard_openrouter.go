package onboarding

import (
	"context"
	"fmt"
	"strings"
)

// This file is the wizard's side of OpenRouter: which methods the provider
// declares, and the steps the operator walks through. The OAuth mechanics it
// starts — the PKCE URL, the callback, the token exchange and the CSRF binding —
// live in openrouter_oauth.go, so a change to the protocol does not re-open the
// onboarding flow and vice versa.

// openrouterAuthHandler implements AuthHandler for OpenRouter (used by opencode agent).
// Supports: api_key (direct key), quick_login (OAuth PKCE).
type openrouterAuthHandler struct {
	wizard *Wizard
}

func (h *openrouterAuthHandler) Methods(_ context.Context) ([]AuthMethod, error) {
	return []AuthMethod{
		{
			ID:          "api_key",
			Name:        "API Key",
			Type:        "env_var",
			Vars:        []string{"OPENROUTER_API_KEY"},
			Description: "Enter your OpenRouter API key directly",
		},
		{
			ID:          "quick_login",
			Name:        "Quick Login (OAuth)",
			Type:        "agent",
			Description: "Authenticate via OpenRouter OAuth in your browser",
		},
	}, nil
}

func (h *openrouterAuthHandler) Authenticate(ctx context.Context, method AuthMethod, input string) (*AuthResult, string, error) {
	switch method.ID {
	case "api_key":
		return h.authenticateAPIKey(ctx, input)
	case "quick_login":
		return h.authenticateOAuth(ctx, input)
	default:
		return nil, "", fmt.Errorf("unknown openrouter auth method: %s", method.ID)
	}
}

func (h *openrouterAuthHandler) authenticateAPIKey(_ context.Context, input string) (*AuthResult, string, error) {
	if input == "" {
		return nil, "Enter your OpenRouter API key:", nil
	}
	return &AuthResult{
		Env: map[string]string{"OPENROUTER_API_KEY": input},
	}, "", nil
}

func (h *openrouterAuthHandler) authenticateOAuth(_ context.Context, input string) (*AuthResult, string, error) {
	if input == "" {
		// Generate auth URL — the verifier is stored in wizard state by the caller
		return nil, "OAUTH_URL_NEEDED", nil
	}
	return nil, "", fmt.Errorf("use HandleAuthCallback for OAuth flow completion")
}

// StartDeclaredMethod implements methodStarter: the OAuth method this handler
// declares is started by this handler's own PKCE flow, and every other method
// stays on the generic path.
func (h *openrouterAuthHandler) StartDeclaredMethod(_ context.Context, method AuthMethod, state *WizardState) (string, bool, error) {
	if method.ID != "quick_login" {
		return "", false, nil
	}
	message, err := h.startOAuth(state)
	return message, true, err
}

// ProviderAuthPrompt implements providerAuthOwner: the prompt of the stage where
// the operator picks how to authenticate against the chosen provider. The
// provider names are data the stage carries in the wizard state, not identities
// this code knows.
func (h *openrouterAuthHandler) ProviderAuthPrompt(state *WizardState) string {
	w := h.wizard
	if state.Context["auth_method"] != "" {
		return ""
	}
	if state.Context["provider"] == "OpenRouter" {
		return w.localizer.GetString(state.Language, "opencode_auth_method_prompt")
	}
	if state.Context["provider"] != "" {
		return fmt.Sprintf(w.localizer.GetString(state.Language, "opencode_api_key_prompt"), state.Context["provider"])
	}
	return ""
}

// HandleProviderAuthInput implements providerAuthOwner: the input of that stage
// belongs to the provider this handler serves.
func (h *openrouterAuthHandler) HandleProviderAuthInput(state *WizardState, input string) (string, bool, error) {
	if state.Context["auth_method"] != "" {
		return "", false, nil
	}
	if state.Context["provider"] == "OpenRouter" {
		response, err := h.handleAuthSelection(state, input)
		return response, true, err
	}
	if state.Context["provider"] != "" {
		response, err := h.handleAPIKey(state, input)
		return response, true, err
	}
	return "", false, nil
}

// startOAuth begins the OpenRouter OAuth PKCE flow for the method this handler
// declares, recording the verifier and the method in the wizard state.
func (h *openrouterAuthHandler) startOAuth(state *WizardState) (string, error) {
	w := h.wizard
	url, verifier, err := h.generateAuthURL(state.Context["channel_id"])
	if err != nil {
		return fmt.Sprintf("⚠️ Could not generate Auth URL: %v", err), nil
	}
	state.Context["pkce_verifier"] = verifier
	state.Context["auth_method"] = "quick_login"
	state.Step = 4
	return fmt.Sprintf(w.localizer.GetString(state.Language, "opencode_openrouter_quick_auth_prompt"), url), nil
}

// handleAuthSelection reads the operator's choice of provider-auth method.
func (h *openrouterAuthHandler) handleAuthSelection(state *WizardState, input string) (string, error) {
	w := h.wizard
	choice, ok := map[string]string{"1": "api_key", "2": "quick_login"}[input]
	if !ok {
		return w.invalidSelection(state, w.promptForStep(*state)), nil
	}
	state.Context["auth_method"] = choice
	if choice != "quick_login" {
		state.Step = 4
		return w.promptForStep(*state), nil
	}
	return h.startOAuth(state)
}

// handleAPIKey records the key entered for a provider that is not OpenRouter.
func (h *openrouterAuthHandler) handleAPIKey(state *WizardState, input string) (string, error) {
	w := h.wizard
	if strings.EqualFold(input, "skip") {
		input = ""
	}
	state.Context["api_key"] = input
	state.Step = 5
	return w.promptForStep(*state), nil
}
