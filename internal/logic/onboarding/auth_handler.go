package onboarding

import (
	"context"

	"github.com/Josepavese/matrix/internal/middleware"
)

// AuthMethod represents a single authentication method declared by an ACP agent
// via the initialize response's authMethods array.
type AuthMethod struct {
	ID          string         `json:"id"`          // Unique method identifier (e.g. "oauth-personal", "codex-api-key")
	Name        string         `json:"name"`        // Human-readable name
	Type        string         `json:"type"`        // "env_var", "terminal", "agent", or "" (agent is default)
	Description string         `json:"description"` // Optional description
	Vars        []string       `json:"vars"`        // For type=env_var: environment variable names
	Args        []string       `json:"args"`        // For type=terminal: command arguments
	Meta        map[string]any `json:"_meta"`       // Extension metadata (terminal-auth, api-key, gateway, etc.)
}

// AuthResult holds the outcome of a successful authentication flow.
// The credentials are passed to session/new via env or headers depending on transport.
type AuthResult struct {
	Env     map[string]string // Environment variable credentials (for stdio transport)
	Headers map[string]string // HTTP headers (for HTTP/SSE transport)
}

// AgentAuthController is the protocol side of agent authentication: the methods
// the agent itself publishes in its initialize response, and the authenticate
// call that executes one of them.
//
// It exists because ACP's authenticate operation was advertised as an available
// capability while the only code able to perform it had no consumer, and the path
// that actually authenticated agents returned a method the agent had never
// claimed. The generic handler consumes this port, which is what makes the
// advertised capability reachable.
type AgentAuthController interface {
	AgentAuthenticationMethods(ctx context.Context, agentID string) ([]middleware.AuthenticationMethod, error)
	AuthenticateAgent(ctx context.Context, agentID, methodID string) error
}

// AuthHandler is the interface for an authentication flow.
// Each agent type (codex, openrouter, generic ACP) implements this.
//
// The flow is:
//  1. Methods() — returns available auth methods (from ACP initialize or hardcoded)
//  2. Resolve(input) — user picks a method; returns the handler to use
//  3. Authenticate(input) — performs the auth flow, returns credentials
type AuthHandler interface {
	// Methods returns the authentication methods this handler supports.
	Methods(ctx context.Context) ([]AuthMethod, error)

	// Authenticate performs the authentication flow for the given method and user input.
	// For interactive flows (device-auth, terminal-login), input may be "" on first call
	// and the return value contains instructions for the user. Subsequent calls include
	// the user's response (e.g. "done").
	Authenticate(ctx context.Context, method AuthMethod, input string) (*AuthResult, string, error)
}

// authHandlerRegistry maps agent IDs to their auth handlers.
// Custom handlers (codex, openrouter) are registered for specific agents.
// Agents without a custom handler use the generic ACP handler.
type authHandlerRegistry struct {
	wizard   *Wizard
	handlers map[string]AuthHandler
	fallback AuthHandler // generic ACP handler
}

func newAuthHandlerRegistry(w *Wizard) *authHandlerRegistry {
	return &authHandlerRegistry{
		wizard: w,
		handlers: map[string]AuthHandler{
			"codex":    &codexAuthHandler{wizard: w},
			"opencode": &openrouterAuthHandler{wizard: w},
		},
		fallback: &acpAuthHandler{wizard: w},
	}
}

func (r *authHandlerRegistry) get(agentID string) AuthHandler {
	if h, ok := r.handlers[agentID]; ok {
		return h
	}
	// The generic handler answers for whichever agent asks it, because the
	// methods it offers come from that agent's own initialize response.
	if generic, ok := r.fallback.(*acpAuthHandler); ok {
		bound := *generic
		bound.agentID = agentID
		return &bound
	}
	return r.fallback
}
