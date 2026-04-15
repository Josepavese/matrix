// Package onboarding implements the first-run setup wizard.
package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/agentcatalog"
	"github.com/jose/matrix-v2/internal/logic/agentcfg"
	"github.com/jose/matrix-v2/internal/logic/agentmgr"
	"github.com/jose/matrix-v2/internal/middleware"
)

// WizardState holds the persisted state of an in-progress wizard session.
type WizardState struct {
	Step      int               `json:"step"`
	Language  string            `json:"language"`
	AgentName string            `json:"agent_name"`
	Context   map[string]string `json:"context"` // no omitempty: empty map must be preserved
}

type AgentEntry = agentcatalog.Entry

// WizardDependencies represents the dependencies required by the Onboarding Wizard.
type WizardDependencies struct {
	Storage   middleware.Storage
	Config    middleware.ConfigManager
	Localizer middleware.LocalizationReader
	Proc      middleware.Process
	Installer *agentmgr.Installer
	Discovery agentcatalog.Discovery
	Activator agentcatalog.Activator
	FS        middleware.FS
	Net       middleware.Network
}

// Wizard orchestrates the first-run onboarding flow.
type Wizard struct {
	storage   middleware.Storage
	config    middleware.ConfigManager
	localizer middleware.LocalizationReader
	proc      middleware.Process
	installer *agentmgr.Installer
	discovery agentcatalog.Discovery
	activator agentcatalog.Activator
	fs        middleware.FS
	net       middleware.Network
	handlers  *authHandlerRegistry
}

// NewWizard creates a new onboarding Wizard with the given dependencies.
func NewWizard(deps WizardDependencies) *Wizard {
	w := &Wizard{
		storage:   deps.Storage,
		config:    deps.Config,
		localizer: deps.Localizer,
		proc:      deps.Proc,
		installer: deps.Installer,
		discovery: deps.Discovery,
		activator: deps.Activator,
		fs:        deps.FS,
		net:       deps.Net,
	}
	if w.discovery == nil || w.activator == nil {
		service := agentcatalog.NewService(agentcatalog.Config{
			Storage:   deps.Storage,
			Net:       deps.Net,
			Installer: deps.Installer,
		})
		if w.discovery == nil {
			w.discovery = service
		}
		if w.activator == nil {
			w.activator = service
		}
	}
	w.handlers = newAuthHandlerRegistry(w)
	return w
}

// fetchAgentList retrieves available agents from the configured discovery layer.
func (w *Wizard) fetchAgentList() ([]AgentEntry, error) {
	if w.discovery == nil {
		return w.installedAgents(), nil
	}
	entries, err := w.discovery.List(context.Background())
	if err != nil {
		slog.Warn("agent discovery failed, using installed agents only", "error", err)
		return w.installedAgents(), nil
	}
	return entries, nil
}

// installedAgents returns only locally installed agents as AgentEntry list.
func (w *Wizard) installedAgents() []AgentEntry {
	ids, err := agentcfg.ListMetaIDs(w.storage)
	if err != nil {
		return nil
	}
	entries := make([]AgentEntry, 0, len(ids))
	for _, id := range ids {
		meta, err := agentcfg.LoadMeta(w.storage, id)
		name := id
		desc := ""
		var dt []string
		if err == nil && meta.Name != "" {
			name = meta.Name
			desc = meta.Description
			dt = meta.DistTypes
		}
		entries = append(entries, AgentEntry{
			ID: id, Name: name, Description: desc,
			DistTypes: dt, Installed: true,
		})
	}
	return entries
}

// installedIDSet returns a set of installed agent IDs.
func (w *Wizard) installedIDSet() map[string]bool {
	ids, err := agentcfg.ListMetaIDs(w.storage)
	if err != nil {
		return nil
	}
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// IsConfigured checks the SSOT Vault to see if Matrix has completed First-Run.
func (w *Wizard) IsConfigured() bool {
	data, err := w.storage.Get("system.configured")
	if err == nil && len(data) > 0 {
		var configured bool
		if err := json.Unmarshal(data, &configured); err == nil && configured {
			return true
		}
	}
	return false
}

// WizardStep defines the behavior for a single step in the onboarding process.
type WizardStep struct {
	Prompt func(w *Wizard, state *WizardState) string
	Handle func(w *Wizard, state *WizardState, input string) (string, error)
}

// Process handles state machine transitions for the given channel during First-Run
func (w *Wizard) Process(channelID, input string) (string, error) {
	stateKey := "wizard.state." + channelID
	state, err := w.restoreState(stateKey)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(input)
	slog.Debug("[Wizard] Process called", "channel", channelID, "step", state.Step, "agent", state.AgentName, "input", trimmed)

	if handled, response := w.handleInitialEntry(stateKey, channelID, &state); handled {
		return response, nil
	}
	if handled, response := w.handleBackCommand(stateKey, trimmed, &state); handled {
		return response, nil
	}

	steps := w.getStepRegistry()
	step, ok := steps[state.Step]
	if !ok {
		slog.Error("Wizard: unknown step", "step", state.Step)
		return "⚠️ System State Error: Step " + fmt.Sprint(state.Step) + " not found.", nil
	}

	response, err := step.Handle(w, &state, trimmed)
	if err != nil {
		return response, err
	}
	w.saveState(stateKey, state)
	return response, nil
}

func (w *Wizard) restoreState(stateKey string) (WizardState, error) {
	data, err := w.storage.Get(stateKey)
	if err != nil {
		return WizardState{}, fmt.Errorf("failed to read wizard state: %w", err)
	}

	var state WizardState
	if len(data) > 0 {
		if err := json.Unmarshal(data, &state); err != nil {
			return WizardState{}, fmt.Errorf("failed to load wizard state: %w", err)
		}
	}
	if state.Language == "" {
		state.Language = "en"
	}
	if state.Context == nil {
		state.Context = make(map[string]string)
	}
	return state, nil
}

func (w *Wizard) handleInitialEntry(stateKey, channelID string, state *WizardState) (bool, string) {
	if state.Step != 0 {
		return false, ""
	}
	state.Step = 1
	state.Context["channel_id"] = channelID
	w.saveState(stateKey, *state)
	return true, w.localizer.GetString("en", "language_selection")
}

func (w *Wizard) handleBackCommand(stateKey, input string, state *WizardState) (bool, string) {
	lower := strings.ToLower(input)
	if lower != "back" && lower != "indietro" {
		return false, ""
	}
	if state.Step > 1 {
		state.Step--
		w.clearStateForStep(state)
		w.saveState(stateKey, *state)
	}
	return true, w.promptForStep(*state)
}

// clearStateForStep resets context when browsing backwards
func (w *Wizard) clearStateForStep(state *WizardState) {
	channelID := state.Context["channel_id"]
	switch state.Step {
	case 1:
		state.AgentName = ""
		state.Context = map[string]string{"channel_id": channelID}
	case 2:
		state.AgentName = ""
		state.Context = map[string]string{"channel_id": channelID}
	case 3:
		delete(state.Context, "provider")
		delete(state.Context, "auth_method")
		delete(state.Context, "api_key")
	case 4:
		delete(state.Context, "api_key")
		delete(state.Context, "model")
	}
}

func (w *Wizard) finishConfiguration(state WizardState) (string, error) {
	err := w.configureAgent(state.AgentName, state.Context)
	if err != nil {
		return fmt.Sprintf(w.localizer.GetString(state.Language, "agent_configure_failed"), err), nil
	}

	stateKey := "wizard.state." + state.Context["channel_id"]
	configuredData, err := json.Marshal(true)
	if err != nil {
		return "", err
	}
	if err := w.storage.Set("system.configured", configuredData); err != nil {
		return "", err
	}
	if err := w.storage.Delete(stateKey); err != nil {
		return "", err
	}

	return fmt.Sprintf(w.localizer.GetString(state.Language, "agent_configured_success"), state.AgentName), nil
}

// promptForStep returns the question text for the given state step.
func (w *Wizard) promptForStep(state WizardState) string {
	steps := w.getStepRegistry()
	step, ok := steps[state.Step]
	if !ok {
		return "Unknown step."
	}

	lang := state.Language
	if lang == "" {
		lang = "en"
	}

	hint := ""
	if state.Step > 1 {
		hint = "\n" + w.localizer.GetString(lang, "back_hint")
	}

	return step.Prompt(w, &state) + hint
}

// configureAgent writes the agent configuration to the SSOT vault using context
// collected during the wizard flow. For agents with AuthResult.Env, those take
// precedence over the legacy context-based env mapping.
func (w *Wizard) configureAgent(agentName string, context map[string]string) error {
	var envs []string

	// If the wizard stored AuthResult.Env, use it directly
	if authEnvJSON := context["_auth_env"]; authEnvJSON != "" {
		var authEnv map[string]string
		if err := json.Unmarshal([]byte(authEnvJSON), &authEnv); err == nil {
			for k, v := range authEnv {
				envs = append(envs, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	// Fallback: legacy context-based env mapping
	if len(envs) == 0 {
		if apiKey := context["api_key"]; apiKey != "" {
			envKey := "API_KEY"
			if agentName == "codex" {
				envKey = "OPENAI_API_KEY"
			}
			envs = append(envs, fmt.Sprintf("%s=%s", envKey, apiKey))
		}
		if provider := context["provider"]; provider != "" {
			envs = append(envs, fmt.Sprintf("PROVIDER=%s", provider))
		}
		if model := context["model"]; model != "" {
			envs = append(envs, fmt.Sprintf("MODEL_NAME=%s", model))
		}
	}

	active := true
	return agentcfg.Save(w.storage, agentName, agentcfg.Override{
		Active: &active,
		Env:    envs,
	})
}

// GetLanguage returns the current language for a channel, defaulting to "en".
func (w *Wizard) GetLanguage(channelID string) string {
	if lang := w.readStoredLanguage("channel.language." + channelID); lang != "" {
		return lang
	}
	if lang := w.readWizardStateLanguage(channelID); lang != "" {
		return lang
	}
	if lang := w.readStoredLanguage("system.language"); lang != "" {
		return lang
	}
	return "en"
}

func (w *Wizard) readStoredLanguage(key string) string {
	data, err := w.storage.Get(key)
	if err != nil || len(data) == 0 {
		return ""
	}

	var lang string
	if err := json.Unmarshal(data, &lang); err != nil {
		return ""
	}
	return lang
}

func (w *Wizard) readWizardStateLanguage(channelID string) string {
	data, err := w.storage.Get("wizard.state." + channelID)
	if err != nil || len(data) == 0 {
		return ""
	}

	var state WizardState
	if err := json.Unmarshal(data, &state); err != nil {
		return ""
	}
	return state.Language
}

// GetString retrieves a localized string from the wizard's localizer.
func (w *Wizard) GetString(lang, key string) string {
	return w.localizer.GetString(lang, key)
}
