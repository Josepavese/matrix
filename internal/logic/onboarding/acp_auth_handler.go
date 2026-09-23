package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// acpAuthHandler is the generic AuthHandler for agents that use the ACP protocol.
//
// The methods it offers are the ones the agent itself publishes in its initialize
// response, read through AgentAuthController. It knows how to run three shapes of
// method locally:
//   - env_var:  prompts for the required environment variable(s)
//   - terminal: instructs the user to run a terminal command (from args)
//   - agent:    agent-specific flow described by _meta.terminal-auth or _meta.api-key
//
// The generic API-key method is a fallback for agents that publish no method at
// all, or whose protocol cannot be reached. It is never offered in place of a
// method the agent did advertise, because that would describe something the agent
// never claimed.
type acpAuthHandler struct {
	wizard  *Wizard
	agentID string

	// advertised remembers the ids the agent published, so Authenticate can tell
	// the agent's own method from the generic fallback and only ask the agent to
	// authenticate when the method is its own.
	advertised map[string]bool
}

func (h *acpAuthHandler) Methods(ctx context.Context) ([]AuthMethod, error) {
	advertised := h.agentMethods(ctx)
	if len(advertised) == 0 {
		return []AuthMethod{
			{
				ID:   "api_key",
				Name: "API Key",
				Type: "env_var",
				Vars: []string{"API_KEY"},
			},
		}, nil
	}

	h.advertised = make(map[string]bool, len(advertised))
	methods := make([]AuthMethod, 0, len(advertised))
	for _, method := range advertised {
		h.advertised[method.ID] = true
		methods = append(methods, AuthMethod{
			ID:          method.ID,
			Name:        method.Name,
			Type:        "agent",
			Description: method.Description,
			Meta:        method.Metadata,
		})
	}
	return methods, nil
}

// agentMethods asks the agent what it advertises, and tolerates every reason not
// to know: no controller wired, an agent that is not reachable, a protocol that
// does not answer. Not knowing falls back to the generic method; it never invents
// an agent method.
func (h *acpAuthHandler) agentMethods(ctx context.Context) []middleware.AuthenticationMethod {
	controller := h.controller()
	if controller == nil || h.agentID == "" {
		return nil
	}
	methods, err := controller.AgentAuthenticationMethods(ctx, h.agentID)
	if err != nil {
		return nil
	}
	return methods
}

func (h *acpAuthHandler) controller() AgentAuthController {
	if h.wizard == nil {
		return nil
	}
	return h.wizard.agentAuthController()
}

func (h *acpAuthHandler) Authenticate(ctx context.Context, method AuthMethod, input string) (*AuthResult, string, error) {
	var (
		result *AuthResult
		prompt string
		err    error
	)
	switch method.Type {
	case "env_var":
		result, prompt, err = h.authenticateEnvVar(method, input)
	case "terminal":
		result, prompt, err = h.authenticateTerminal(method, input)
	default:
		result, prompt, err = h.authenticateAgent(method, input)
	}
	if err != nil || prompt != "" || result == nil {
		return result, prompt, err
	}

	// The local step is finished. When the method is the agent's own, the
	// authorization itself happens over the protocol: authenticate(methodID) is
	// what the agent acts on, so a failure here is reported instead of being
	// dressed up as a successful login.
	if h.advertised[method.ID] {
		controller := h.controller()
		if controller != nil {
			if authErr := controller.AuthenticateAgent(ctx, h.agentID, method.ID); authErr != nil {
				return nil, "", fmt.Errorf("agent rejected authenticate(%q): %w", method.ID, authErr)
			}
		}
	}
	return result, "", nil
}

// authenticateEnvVar handles type=env_var methods (e.g. OPENAI_API_KEY, CODEX_API_KEY).
// First call (input="") prompts for the key. Second call receives the key.
func (h *acpAuthHandler) authenticateEnvVar(method AuthMethod, input string) (*AuthResult, string, error) {
	if input == "" {
		varNames := method.Vars
		if len(varNames) == 0 {
			varNames = []string{"API_KEY"}
		}
		prompt := fmt.Sprintf("Enter your %s:", strings.Join(varNames, " / "))
		if method.Description != "" {
			prompt = method.Description + "\n" + prompt
		}
		return nil, prompt, nil
	}

	env := make(map[string]string)
	for _, v := range method.Vars {
		if len(method.Vars) == 1 {
			env[v] = input
		} else {
			// Multiple vars: input is comma-separated "KEY1=val1,KEY2=val2"
			env[v] = "" // will be filled from parsed input
		}
	}
	if len(method.Vars) == 1 {
		env[method.Vars[0]] = input
	} else {
		pairs := strings.Split(input, ",")
		for _, pair := range pairs {
			kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(kv) == 2 {
				env[kv[0]] = kv[1]
			}
		}
	}

	return &AuthResult{Env: env}, "", nil
}

// authenticateTerminal handles type=terminal methods.
// Shows the user a command to run, waits for "done".
func (h *acpAuthHandler) authenticateTerminal(method AuthMethod, input string) (*AuthResult, string, error) {
	if input == "" {
		cmd := buildTerminalCommand(method)
		prompt := fmt.Sprintf("Run this command in your terminal:\n\n  %s\n\nReply 'done' when complete.", cmd)
		if method.Description != "" {
			prompt = method.Description + "\n\n" + prompt
		}
		return nil, prompt, nil
	}

	if strings.EqualFold(input, "done") || strings.EqualFold(input, "fatto") {
		// Terminal auth methods typically store credentials themselves
		return &AuthResult{}, "", nil
	}

	return nil, "Reply 'done' when you have completed the authentication.", nil
}

// authenticateAgent handles type=agent methods using _meta extensions.
// Supports:
//   - _meta.terminal-auth: shows command from {command, args, label}
//   - _meta.api-key: prompts for API key with provider info
//   - Default: generic instruction
func (h *acpAuthHandler) authenticateAgent(method AuthMethod, input string) (*AuthResult, string, error) {
	if method.Meta != nil {
		if ta, ok := method.Meta["terminal-auth"]; ok {
			return h.handleTerminalAuthMeta(method, ta, input)
		}
		if ak, ok := method.Meta["api-key"]; ok {
			return h.handleAPIKeyMeta(method, ak, input)
		}
	}

	// Default: just instruct the user to authenticate and come back
	if input == "" {
		instruction := fmt.Sprintf("Authenticate with %s and reply 'done' when complete.", method.Name)
		if method.Description != "" {
			instruction = method.Description + "\n\n" + instruction
		}
		return nil, instruction, nil
	}

	if strings.EqualFold(input, "done") || strings.EqualFold(input, "fatto") {
		return &AuthResult{}, "", nil
	}

	return nil, "Reply 'done' when you have completed authentication.", nil
}

func (h *acpAuthHandler) handleTerminalAuthMeta(method AuthMethod, meta any, input string) (*AuthResult, string, error) {
	// _meta.terminal-auth is typically: {"command": "/path/to/binary", "args": ["login"], "label": "Login"}
	cmd := buildCommandFromMeta(meta)
	if input == "" {
		prompt := fmt.Sprintf("Run this command in your terminal:\n\n  %s\n\nReply 'done' when complete.", cmd)
		if method.Description != "" {
			prompt = method.Description + "\n\n" + prompt
		}
		return nil, prompt, nil
	}

	if strings.EqualFold(input, "done") || strings.EqualFold(input, "fatto") {
		return &AuthResult{}, "", nil
	}
	return nil, "Reply 'done' when you have completed authentication.", nil
}

func (h *acpAuthHandler) handleAPIKeyMeta(method AuthMethod, meta any, input string) (*AuthResult, string, error) {
	// _meta.api-key is typically: {"provider": "google"} or similar
	provider := ""
	if m, ok := meta.(map[string]any); ok {
		if p, ok := m["provider"].(string); ok {
			provider = p
		}
	}

	if input == "" {
		prompt := "Enter your API key"
		if provider != "" {
			prompt = fmt.Sprintf("Enter your %s API key", provider)
		}
		prompt += ":"
		if method.Description != "" {
			prompt = method.Description + "\n" + prompt
		}
		return nil, prompt, nil
	}

	envVar := "API_KEY"
	if len(method.Vars) > 0 {
		envVar = method.Vars[0]
	}
	return &AuthResult{
		Env: map[string]string{envVar: input},
	}, "", nil
}

// buildTerminalCommand constructs a display command from a terminal auth method.
func buildTerminalCommand(method AuthMethod) string {
	if len(method.Args) > 0 {
		return method.ID + " " + strings.Join(method.Args, " ")
	}
	return method.ID
}

// buildCommandFromMeta extracts a command string from _meta.terminal-auth.
func buildCommandFromMeta(meta any) string {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Sprintf("%v", meta)
	}
	var ta struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Label   string   `json:"label"`
	}
	if err := json.Unmarshal(data, &ta); err != nil {
		return string(data)
	}
	if ta.Command != "" {
		cmd := ta.Command
		if len(ta.Args) > 0 {
			cmd += " " + strings.Join(ta.Args, " ")
		}
		return cmd
	}
	return ta.Label
}
