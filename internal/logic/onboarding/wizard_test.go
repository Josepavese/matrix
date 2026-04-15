package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jose/matrix-v2/internal/logic/agentcatalog"
	"github.com/jose/matrix-v2/internal/logic/agentcfg"
	"github.com/jose/matrix-v2/internal/logic/agentdiscovery"
	"github.com/jose/matrix-v2/internal/middleware"
)

// --- Test mocks ---

type testStorage struct {
	data map[string][]byte
}

func newTestStorage() *testStorage {
	return &testStorage{data: make(map[string][]byte)}
}

func (s *testStorage) Get(key string) ([]byte, error) {
	return s.data[key], nil
}

func (s *testStorage) Set(key string, val []byte) error {
	s.data[key] = val
	return nil
}

func (s *testStorage) Delete(key string) error {
	delete(s.data, key)
	return nil
}

func (s *testStorage) List(prefix string) ([]string, error) {
	var keys []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

type testLocalizer struct {
	strings map[string]map[string]string
}

func newTestLocalizer() *testLocalizer {
	return &testLocalizer{
		strings: map[string]map[string]string{
			"en": {
				"language_selection":                    "Select language:\n1. English\n2. Italiano",
				"agent_selection_header":                "Select the agent to connect:\n",
				"agent_installed_mark":                  "✅",
				"agent_installing":                      "Installing %s...",
				"agent_install_failed":                  "Could not install %s: %v",
				"generic_api_key_prompt":                "Enter API key for %s:",
				"codex_auth_method_prompt":              "Codex auth:\n1. ChatGPT Login\n2. OpenAI API Key",
				"codex_openai_key_prompt":               "Enter your OpenAI API key:",
				"opencode_provider_prompt":              "Select provider:\n1. OpenRouter\n2. Local/Ollama",
				"opencode_auth_method_prompt":           "OpenRouter auth:\n1. API Key\n2. Quick Login",
				"opencode_api_key_prompt":               "Enter %s API key:",
				"opencode_model_prompt":                 "Enter model name:",
				"opencode_openrouter_quick_auth_prompt": "Open this URL: %s",
				"agent_configure_failed":                "Failed: %v",
				"agent_configured_success":              "%s configured!",
				"invalid_selection_number":              "Invalid selection.",
				"back_hint":                             "Type 'back' to go back.",
			},
			"it": {
				"language_selection": "Scegli lingua:\n1. English\n2. Italiano",
			},
		},
	}
}

func (l *testLocalizer) GetString(lang, key string) string {
	if m, ok := l.strings[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

type testFS struct{}

func (f *testFS) Mount(string) error                   { return nil }
func (f *testFS) Unmount() error                       { return nil }
func (f *testFS) CreateDirectory(string) error         { return nil }
func (f *testFS) RemoveAll(string) error               { return nil }
func (f *testFS) Stat(string) (os.FileInfo, error)     { return nil, os.ErrNotExist }
func (f *testFS) MkdirAll(string, os.FileMode) error   { return nil }
func (f *testFS) UserHomeDir() (string, error)         { return "/tmp", nil }
func (f *testFS) TempDir() string                      { return "/tmp" }
func (f *testFS) Open(string) (middleware.File, error) { return nil, os.ErrNotExist }
func (f *testFS) OpenFile(string, int, os.FileMode) (middleware.File, error) {
	return nil, os.ErrNotExist
}
func (f *testFS) ReadFile(string) ([]byte, error) { return nil, os.ErrNotExist }
func (f *testFS) Remove(string) error             { return nil }
func (f *testFS) Rename(_, _ string) error        { return nil }

type testNet struct{}

func (n *testNet) Listen(_, _ string) (middleware.ClosableListener, error) { return nil, nil }
func (n *testNet) Download(_ context.Context, _, _ string) error           { return nil }
func (n *testNet) FetchJSON(_ context.Context, _ string, _ interface{}) error {
	return fmt.Errorf("not implemented")
}
func (n *testNet) GetFreePort() (int, error) { return 0, nil }
func (n *testNet) Fetch(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}
func (n *testNet) PostJSON(_ context.Context, _ string, _ interface{}) ([]byte, int, error) {
	return nil, 0, fmt.Errorf("not implemented")
}
func (n *testNet) CanDial(_ string) bool { return false }

type testDiscovery struct {
	entries []agentcatalog.Entry
	err     error
}

func (d *testDiscovery) List(context.Context) ([]agentcatalog.Entry, error) {
	return append([]agentcatalog.Entry{}, d.entries...), d.err
}

type testActivator struct {
	calls []agentcatalog.Entry
	err   error
}

func (a *testActivator) Activate(_ context.Context, entry agentcatalog.Entry) error {
	a.calls = append(a.calls, entry)
	return a.err
}

func newTestWizard() *Wizard {
	store := newTestStorage()
	w := NewWizard(WizardDependencies{
		Storage:   store,
		Localizer: newTestLocalizer(),
		FS:        &testFS{},
		Net:       &testNet{},
	})
	return w
}

// newTestWizardWithAgents creates a wizard with pre-populated agents in the vault.
func newTestWizardWithAgents() *Wizard {
	w := newTestWizard()
	// Pre-populate some agents in the vault (simulating installed agents)
	agents := []agentcfg.Meta{
		{ID: "opencode", Name: "OpenCode", Description: "Local code agent", DistTypes: []string{"binary"}},
		{ID: "gemini", Name: "Gemini", Description: "Google AI agent", DistTypes: []string{"npx"}},
		{ID: "claude-acp", Name: "Claude Agent", Description: "Anthropic AI agent", DistTypes: []string{"npx"}},
		{ID: "kimi", Name: "Kimi", Description: "Moonshot agent", DistTypes: []string{"npx"}},
		{ID: "codex", Name: "Codex", Description: "OpenAI code agent", DistTypes: []string{"binary", "npx"}},
	}
	for _, a := range agents {
		if err := agentcfg.SaveMeta(w.storage, a.ID, a); err != nil {
			panic(err)
		}
	}
	return w
}

// --- Tests ---

func TestWizard_LanguageSelection(t *testing.T) {
	w := newTestWizard()
	resp, err := w.Process("ch1", "")
	if err != nil {
		t.Fatalf("step 0: %v", err)
	}
	if !strings.Contains(resp, "Select language") && !strings.Contains(resp, "language_selection") {
		t.Fatalf("expected language selection prompt, got: %s", resp)
	}
}

func TestWizard_GenericAgent_APIKey(t *testing.T) {
	w := newTestWizardWithAgents()

	// Step 0->1: Initial entry
	resp, err := w.Process("ch2", "")
	_ = resp
	if err != nil {
		t.Fatalf("step 0: %v", err)
	}

	// Step 1->2: Select English
	resp, err = w.Process("ch2", "1")
	if err != nil {
		t.Fatalf("step 1: %v", err)
	}
	if !strings.Contains(resp, "Gemini") && !strings.Contains(resp, "agent") {
		t.Fatalf("expected agent selection, got: %s", resp)
	}

	// Agents sorted: claude-acp(1), codex-acp(2), gemini(3), kimi(4), opencode(5)
	// Select gemini (index 3)
	resp, err = w.Process("ch2", "3")
	if err != nil {
		t.Fatalf("step 2: %v", err)
	}
	// Gemini has no custom handler, acpAuthHandler returns 1 method (generic API key)
	// Single method → prompt should be from Authenticate with empty input
	if !strings.Contains(resp, "API") && !strings.Contains(resp, "Key") && !strings.Contains(resp, "api_key") {
		t.Fatalf("expected API key prompt for generic agent, got: %s", resp)
	}

	// Step 3->finish: Enter API key
	resp, err = w.Process("ch2", "sk-test-123")
	if err != nil {
		t.Fatalf("step 3 (api key): %v", err)
	}
	if !strings.Contains(resp, "gemini") && !strings.Contains(resp, "configured") {
		t.Fatalf("expected configuration success, got: %s", resp)
	}

	// Verify agent config was saved
	configuredData, _ := w.storage.Get("system.configured")
	if string(configuredData) != "true" {
		t.Fatal("expected system.configured = true")
	}
}

func TestWizard_Codex_APIKey(t *testing.T) {
	w := newTestWizardWithAgents()

	// Step 0->1
	_, _ = w.Process("ch3", "")

	// Step 1->2: Select English
	_, _ = w.Process("ch3", "1")

	// Step 2->3: Select codex (index 2 in alphabetical order)
	resp, err := w.Process("ch3", "2")
	if err != nil {
		t.Fatalf("step 2 (codex): %v", err)
	}

	// With AuthHandler, codex has 3 methods (chatgpt, openai-api-key, codex-api-key)
	// step3Prompt shows selection menu since len(methods) > 1
	if !strings.Contains(resp, "ChatGPT") && !strings.Contains(resp, "API Key") {
		t.Fatalf("expected codex auth method selection, got: %s", resp)
	}

	// Select method 2 (openai-api-key)
	resp, err = w.Process("ch3", "2")
	if err != nil {
		t.Fatalf("step 3 (select method): %v", err)
	}
	// Should prompt for the key
	if !strings.Contains(resp, "OPENAI_API_KEY") {
		t.Fatalf("expected OPENAI_API_KEY prompt, got: %s", resp)
	}

	// Enter API key
	resp, err = w.Process("ch3", "sk-openai-test-key")
	if err != nil {
		t.Fatalf("step 4 (enter key): %v", err)
	}
	if !strings.Contains(resp, "codex") && !strings.Contains(resp, "configured") {
		t.Fatalf("expected codex configured success, got: %s", resp)
	}
}

func TestWizard_BackNavigation(t *testing.T) {
	w := newTestWizardWithAgents()

	// Navigate to step 2
	_, _ = w.Process("ch4", "")
	_, _ = w.Process("ch4", "1")

	// Select an agent
	_, _ = w.Process("ch4", "2")

	// Go back
	resp, err := w.Process("ch4", "back")
	if err != nil {
		t.Fatalf("back: %v", err)
	}
	// Should show agent selection again
	if !strings.Contains(resp, "Gemini") && !strings.Contains(resp, "agent") {
		t.Fatalf("expected agent selection after back, got: %s", resp)
	}
}

func TestWizard_AuthHandlerRegistry(t *testing.T) {
	w := newTestWizard()
	reg := w.handlers

	// Codex has custom handler
	h := reg.get("codex")
	if _, ok := h.(*codexAuthHandler); !ok {
		t.Fatal("expected codexAuthHandler for codex")
	}

	// OpenCode has custom handler
	h = reg.get("opencode")
	if _, ok := h.(*openrouterAuthHandler); !ok {
		t.Fatal("expected openrouterAuthHandler for opencode")
	}

	// Unknown agent gets generic ACP handler
	h = reg.get("gemini")
	if _, ok := h.(*acpAuthHandler); !ok {
		t.Fatal("expected acpAuthHandler for gemini")
	}

	h = reg.get("some-random-agent")
	if _, ok := h.(*acpAuthHandler); !ok {
		t.Fatal("expected acpAuthHandler fallback for unknown agent")
	}
}

func TestACPAuthHandler_EnvVar(t *testing.T) {
	w := newTestWizard()
	h := &acpAuthHandler{wizard: w}

	methods, err := h.Methods(context.Background())
	if err != nil {
		t.Fatalf("Methods: %v", err)
	}
	if len(methods) != 1 || methods[0].Type != "env_var" {
		t.Fatalf("expected 1 env_var method, got: %+v", methods)
	}

	// First call: prompt for key
	result, prompt, err := h.Authenticate(context.Background(), methods[0], "")
	if err != nil {
		t.Fatalf("Authenticate (prompt): %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result on first call")
	}
	if !strings.Contains(prompt, "API_KEY") {
		t.Fatalf("expected API_KEY prompt, got: %s", prompt)
	}

	// Second call: provide key
	result, prompt, err = h.Authenticate(context.Background(), methods[0], "my-secret-key")
	if err != nil {
		t.Fatalf("Authenticate (key): %v", err)
	}
	if prompt != "" {
		t.Fatalf("expected empty prompt on completion, got: %s", prompt)
	}
	if result.Env["API_KEY"] != "my-secret-key" {
		t.Fatalf("expected API_KEY=my-secret-key, got: %+v", result.Env)
	}
}

func TestACPAuthHandler_TerminalAuth(t *testing.T) {
	w := newTestWizard()
	h := &acpAuthHandler{wizard: w}

	method := AuthMethod{
		ID:   "pi_terminal_login",
		Name: "Terminal Login",
		Type: "terminal",
		Args: []string{"--terminal-login"},
	}

	// First call: show command
	result, prompt, err := h.Authenticate(context.Background(), method, "")
	if err != nil {
		t.Fatalf("Authenticate (prompt): %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result on first call")
	}
	if !strings.Contains(prompt, "--terminal-login") {
		t.Fatalf("expected terminal command in prompt, got: %s", prompt)
	}

	// Second call: user says done
	result, prompt, err = h.Authenticate(context.Background(), method, "done")
	if err != nil {
		t.Fatalf("Authenticate (done): %v", err)
	}
	if prompt != "" {
		t.Fatalf("expected empty prompt, got: %s", prompt)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestACPAuthHandler_AgentMeta_TerminalAuth(t *testing.T) {
	w := newTestWizard()
	h := &acpAuthHandler{wizard: w}

	method := AuthMethod{
		ID:   "copilot-login",
		Name: "Copilot Login",
		Type: "agent",
		Meta: map[string]any{
			"terminal-auth": map[string]any{
				"command": "/usr/local/bin/copilot",
				"args":    []any{"login"},
				"label":   "Login to GitHub Copilot",
			},
		},
	}

	// First call: show command from _meta
	_, prompt, err := h.Authenticate(context.Background(), method, "")
	if err != nil {
		t.Fatalf("Authenticate (prompt): %v", err)
	}
	if !strings.Contains(prompt, "copilot") && !strings.Contains(prompt, "login") {
		t.Fatalf("expected copilot login command in prompt, got: %s", prompt)
	}

	// Second call: done
	result, _, err := h.Authenticate(context.Background(), method, "done")
	if err != nil {
		t.Fatalf("Authenticate (done): %v", err)
	}
	if result == nil {
		t.Fatal("expected result on done")
	}
}

func TestACPAuthHandler_AgentMeta_APIKey(t *testing.T) {
	w := newTestWizard()
	h := &acpAuthHandler{wizard: w}

	method := AuthMethod{
		ID:   "gemini-api-key",
		Name: "Gemini API Key",
		Type: "agent",
		Meta: map[string]any{
			"api-key": map[string]any{
				"provider": "google",
			},
		},
	}

	// First call: prompt for key
	_, prompt, err := h.Authenticate(context.Background(), method, "")
	if err != nil {
		t.Fatalf("Authenticate (prompt): %v", err)
	}
	if !strings.Contains(prompt, "google") {
		t.Fatalf("expected google in prompt, got: %s", prompt)
	}

	// Second call: provide key
	result, _, err := h.Authenticate(context.Background(), method, "AIza-sy-test-key")
	if err != nil {
		t.Fatalf("Authenticate (key): %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.Env["API_KEY"] != "AIza-sy-test-key" {
		t.Fatalf("expected API_KEY env, got: %+v", result.Env)
	}
}

func TestCodexAuthHandler_Methods(t *testing.T) {
	w := newTestWizard()
	h := &codexAuthHandler{wizard: w}

	methods, err := h.Methods(context.Background())
	if err != nil {
		t.Fatalf("Methods: %v", err)
	}
	if len(methods) != 3 {
		t.Fatalf("expected 3 methods, got %d", len(methods))
	}

	ids := make(map[string]bool)
	for _, m := range methods {
		ids[m.ID] = true
	}
	if !ids["chatgpt"] || !ids["openai-api-key"] || !ids["codex-api-key"] {
		t.Fatalf("expected chatgpt, openai-api-key, codex-api-key; got: %+v", methods)
	}
}

func TestCodexAuthHandler_APIKey(t *testing.T) {
	w := newTestWizard()
	h := &codexAuthHandler{wizard: w}

	method := AuthMethod{ID: "openai-api-key", Type: "env_var", Vars: []string{"OPENAI_API_KEY"}}

	// Prompt
	_, prompt, err := h.Authenticate(context.Background(), method, "")
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if !strings.Contains(prompt, "OPENAI_API_KEY") {
		t.Fatalf("expected OPENAI_API_KEY in prompt, got: %s", prompt)
	}

	// Provide key
	result, _, err := h.Authenticate(context.Background(), method, "sk-test")
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	if result.Env["OPENAI_API_KEY"] != "sk-test" {
		t.Fatalf("expected OPENAI_API_KEY=sk-test, got: %+v", result.Env)
	}
}

func TestOpenRouterAuthHandler_Methods(t *testing.T) {
	w := newTestWizard()
	h := &openrouterAuthHandler{wizard: w}

	methods, err := h.Methods(context.Background())
	if err != nil {
		t.Fatalf("Methods: %v", err)
	}
	if len(methods) != 2 {
		t.Fatalf("expected 2 methods, got %d", len(methods))
	}
	if methods[0].ID != "api_key" || methods[1].ID != "quick_login" {
		t.Fatalf("expected api_key and quick_login, got: %+v", methods)
	}
}

func TestOpenRouterAuthHandler_APIKey(t *testing.T) {
	w := newTestWizard()
	h := &openrouterAuthHandler{wizard: w}

	method := AuthMethod{ID: "api_key", Type: "env_var", Vars: []string{"OPENROUTER_API_KEY"}}

	result, _, err := h.Authenticate(context.Background(), method, "or-key-123")
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	if result.Env["OPENROUTER_API_KEY"] != "or-key-123" {
		t.Fatalf("expected OPENROUTER_API_KEY, got: %+v", result.Env)
	}
}

func TestWizard_InstallFailureResetsState(t *testing.T) {
	// Test the step2Handle logic directly: when an agent that is NOT installed
	// is selected but there's no installer, the state should remain valid.
	// The real install-failure path is tested via integration tests since
	// Installer is a concrete type.
	store := newTestStorage()
	w := NewWizard(WizardDependencies{
		Storage:   store,
		Localizer: newTestLocalizer(),
		FS:        &testFS{},
		Net:       &testNet{},
		// No installer — agents won't auto-install
	})

	channelID := "test-fail-ch"

	// Step 0->1: Start wizard
	_, err := w.Process(channelID, "hi")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// Step 1->2: Select English
	_, err = w.Process(channelID, "1")
	if err != nil {
		t.Fatalf("language: %v", err)
	}

	// Inject agent list: one not-installed, one installed
	agents := []AgentEntry{
		{ID: "not-installed", Name: "NotInstalled", Description: "Not there", Installed: false},
		{ID: "opencode", Name: "OpenCode", Description: "Already there", Installed: true},
	}
	// Also pre-populate agentcfg meta so installedAgents() works
	if err := agentcfg.SaveMeta(store, "opencode", agentcfg.Meta{
		ID: "opencode", Name: "OpenCode", Description: "Already there",
	}); err != nil {
		t.Fatal(err)
	}

	agentData, _ := json.Marshal(agents)
	stateKey := "wizard.state." + channelID
	stateData, _ := store.Get(stateKey)
	var state WizardState
	_ = json.Unmarshal(stateData, &state)
	state.Context["_agent_list"] = string(agentData)
	stateData, _ = json.Marshal(state)
	_ = store.Set(stateKey, stateData)

	// Step 2: Select not-installed agent (index 1) — no installer, proceeds anyway
	resp, err := w.Process(channelID, "1")
	if err != nil {
		t.Fatalf("not-installed: %v", err)
	}
	// Without installer, agent is selected anyway and advances to step 3
	stateData, _ = store.Get(stateKey)
	_ = json.Unmarshal(stateData, &state)
	if state.AgentName != "not-installed" {
		t.Errorf("AgentName should be 'not-installed', got: %q", state.AgentName)
	}
	if state.Step != 3 {
		t.Errorf("Step should be 3, got: %d", state.Step)
	}
	_ = resp
}

func TestWizard_FetchAgentList_UsesDiscoveryLayer(t *testing.T) {
	w := NewWizard(WizardDependencies{
		Storage:   newTestStorage(),
		Localizer: newTestLocalizer(),
		Discovery: &testDiscovery{
			entries: []agentcatalog.Entry{
				{ID: "codex", Name: "Codex", Source: agentdiscovery.SourceACPRegistry, Kind: middleware.ProtocolKindACP},
				{ID: "remote-planner", Name: "Remote Planner", Source: agentdiscovery.SourceA2ACatalog, Kind: middleware.ProtocolKindA2A, Transport: "JSONRPC"},
			},
		},
		FS:  &testFS{},
		Net: &testNet{},
	})

	agents, err := w.fetchAgentList()
	if err != nil {
		t.Fatalf("fetchAgentList: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
	if agents[1].Source != agentdiscovery.SourceA2ACatalog {
		t.Fatalf("expected A2A catalog source, got %q", agents[1].Source)
	}
}

func TestWizard_Step2Handle_ActivatesDiscoveredAgent(t *testing.T) {
	store := newTestStorage()
	activator := &testActivator{}
	w := NewWizard(WizardDependencies{
		Storage:   store,
		Localizer: newTestLocalizer(),
		Activator: activator,
		FS:        &testFS{},
		Net:       &testNet{},
	})

	channelID := "activate-a2a"
	_, _ = w.Process(channelID, "")
	_, _ = w.Process(channelID, "1")

	agents := []AgentEntry{
		{
			ID:        "remote-planner",
			Name:      "Remote Planner",
			Source:    agentdiscovery.SourceA2ACatalog,
			Kind:      middleware.ProtocolKindA2A,
			Transport: "JSONRPC",
			Address:   "http://127.0.0.1:8088/a2a",
			Installed: false,
		},
	}
	agentData, _ := json.Marshal(agents)
	stateKey := "wizard.state." + channelID
	stateData, _ := store.Get(stateKey)
	var state WizardState
	_ = json.Unmarshal(stateData, &state)
	state.Context["_agent_list"] = string(agentData)
	stateData, _ = json.Marshal(state)
	_ = store.Set(stateKey, stateData)

	resp, err := w.Process(channelID, "1")
	if err != nil {
		t.Fatalf("process step2: %v", err)
	}
	if len(activator.calls) != 1 {
		t.Fatalf("expected 1 activation call, got %d", len(activator.calls))
	}
	if activator.calls[0].ID != "remote-planner" {
		t.Fatalf("expected remote-planner activation, got %+v", activator.calls[0])
	}
	if !strings.Contains(resp, "Installing Remote Planner") {
		t.Fatalf("expected activation status message, got: %s", resp)
	}
}

func TestConfigureAgent_AuthResultEnv(t *testing.T) {
	w := newTestWizard()
	state := WizardState{
		Step:      4,
		Language:  "en",
		AgentName: "gemini",
		Context: map[string]string{
			"channel_id": "test-ch",
			"_auth_env":  `{"API_KEY":"test-key-123","GOOGLE_API_KEY":"test-key-123"}`,
		},
	}

	resp, err := w.finishConfiguration(state)
	if err != nil {
		t.Fatalf("finishConfiguration: %v", err)
	}
	if !strings.Contains(resp, "gemini") {
		t.Fatalf("expected success message with 'gemini', got: %s", resp)
	}

	// Verify the env was stored in agent config
	data, _ := w.storage.Get("agent.config.gemini")
	if data == nil {
		t.Fatal("expected agent.config.gemini to be set")
	}
	fmt.Printf("Agent config: %s\n", string(data))
}
