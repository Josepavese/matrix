package agentlaunch

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// ACPSessionModeEnv is the neutral agent-launch contract for a required ACP
// session mode. Its value is the exact mode identifier the provider must apply
// to a session, not a partial match, a prefix or a prompt suggestion.
//
// The contract belongs to the endpoint that publishes it, so it is role-scoped
// in the only way a provider can be: an agent's own configuration carries it,
// and Matrix applies it to the sessions of that agent. It is deliberately not
// inferred from the daemon's environment or from the agent's name.
const ACPSessionModeEnv = "MATRIX_ACP_SESSION_MODE"

const (
	// sessionModeEnforcementRequired says the mode is a contract, not a
	// preference: Matrix must apply and confirm it or fail the session before
	// the prompt leaves the daemon.
	sessionModeEnforcementRequired = "required"
	// sessionModeApplicationMechanism names the ACP surface the contract is
	// applied through. Both the config-option and the session-mode surface are
	// ACP session mode selection, so the mechanism is the same and the adapter
	// records which response confirmed it.
	sessionModeApplicationMechanism = "acp-session-mode"
	// sessionModeVerificationPending means launch resolution recorded the
	// contract, not its application: only a provider session can confirm that
	// the mode was applied, so the launch evidence never claims it.
	sessionModeVerificationPending = "pending_provider_application"
	// sessionModeRejected means the launch contract itself was invalid, so no
	// provider session may be created under it.
	sessionModeRejected = "rejected_launch_contract"
)

// acpSessionModePattern is the accepted identifier shape. A session mode is a
// short token a provider uses as an option value, so the contract accepts the
// same conservative shape for every provider instead of guessing per-provider
// rules. A value that does not match is refused rather than normalized.
var acpSessionModePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// sessionModeAdapter governs endpoints that publish the neutral session mode
// contract.
//
// Matches reads only the presence of the contract variable, including a
// malformed or duplicated declaration: a launch that asks for the contract must
// be resolved by it, so an invalid declaration is refused loudly instead of
// passing the variable to a provider that never read it.
type sessionModeAdapter struct{}

func (sessionModeAdapter) Matches(endpoint middleware.ProtocolEndpoint) bool {
	return hasEnvName(endpoint.Env, ACPSessionModeEnv)
}

func (sessionModeAdapter) Resolve(endpoint middleware.ProtocolEndpoint) (Resolution, error) {
	result := Resolution{Endpoint: endpoint}
	mode, err := resolveRequiredACPSessionMode(endpoint)
	if err != nil {
		result.Metadata = sessionModeMetadata("", sessionModeRejected)
		return result, err
	}
	result.Metadata = sessionModeMetadata(mode, sessionModeVerificationPending)
	return result, nil
}

// RequiredSessionMode returns the validated required session mode an endpoint
// published, or "" when it published none, a malformed one, or one that
// conflicts with another mode contract.
//
// The ACP adapter uses it to enforce the contract on every session it creates or
// resumes. A malformed declaration is empty here because ResolveEndpoint already
// refused it before the client was built; the endpoint contract remains the one
// place that decides validity.
func RequiredSessionMode(endpoint middleware.ProtocolEndpoint) string {
	if !hasEnvName(endpoint.Env, ACPSessionModeEnv) {
		return ""
	}
	mode, err := resolveRequiredACPSessionMode(endpoint)
	if err != nil {
		return ""
	}
	return mode
}

func resolveRequiredACPSessionMode(endpoint middleware.ProtocolEndpoint) (string, error) {
	values := envValues(endpoint.Env, ACPSessionModeEnv)
	switch len(values) {
	case 0:
		return "", nil
	case 1:
	default:
		return "", fmt.Errorf("%s must be declared once per endpoint, found %d entries", ACPSessionModeEnv, len(values))
	}
	mode := values[0]
	if mode == "" {
		return "", fmt.Errorf("%s requires a session mode identifier", ACPSessionModeEnv)
	}
	if !acpSessionModePattern.MatchString(mode) {
		return "", fmt.Errorf("%s value %q is not a valid ACP session mode identifier (%s)", ACPSessionModeEnv, mode, acpSessionModePattern.String())
	}
	if endpoint.Kind != middleware.ProtocolKindACP {
		return "", fmt.Errorf("%s applies to ACP endpoints only, not to protocol kind %q", ACPSessionModeEnv, endpoint.Kind)
	}
	if owner, declared := codexOwnedSessionMode(endpoint); declared {
		if owner != "" {
			return "", fmt.Errorf("%s=%q conflicts with the Codex launch-policy contract, which already requires session mode %q", ACPSessionModeEnv, mode, owner)
		}
		return "", fmt.Errorf("%s=%q conflicts with the Codex launch-policy contract declared on the same endpoint", ACPSessionModeEnv, mode)
	}
	return mode, nil
}

// codexOwnedSessionMode reports whether the Codex launch-policy contract is
// declared on this endpoint and, when that declaration owns a session mode,
// which mode it requires. The declaration may take any of the contract's forms:
// the contract env, the initial mode env, policy argv or CODEX_CONFIG.
//
// It exists so two contracts that could both own the session mode cannot
// silently disagree: the neutral contract refuses instead of letting one win by
// adapter order.
func codexOwnedSessionMode(endpoint middleware.ProtocolEndpoint) (string, bool) {
	state, err := readCodexPolicyState(endpoint)
	if err != nil {
		return "", true
	}
	if !state.active() {
		return "", false
	}
	mode, _, _ := resolveCodexMode(state.policy, state.mode)
	return mode, true
}

func sessionModeMetadata(mode, status string) map[string]interface{} {
	contract := map[string]interface{}{
		"enforcement":           sessionModeEnforcementRequired,
		"application_mechanism": sessionModeApplicationMechanism,
		"verification_status":   status,
		"verified":              false,
	}
	if mode != "" {
		contract["configured"] = mode
	}
	return map[string]interface{}{"session_mode": contract}
}

// hasEnvName reports whether an environment slice declares the exact name. The
// comparison is on the name, not on the rendered entry: a differently spelled
// variable is not this contract speaking.
func hasEnvName(env []string, key string) bool {
	for _, entry := range env {
		if name, _, ok := strings.Cut(entry, "="); ok && name == key {
			return true
		}
	}
	return false
}

// envValues returns every value declared for the exact name, in order. It keeps
// duplicates visible so the contract can refuse them instead of silently
// letting the last entry win.
func envValues(env []string, key string) []string {
	var values []string
	for _, entry := range env {
		if name, value, ok := strings.Cut(entry, "="); ok && name == key {
			values = append(values, value)
		}
	}
	return values
}
