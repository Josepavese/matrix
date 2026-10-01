// Package agentlaunch resolves governed provider launch policy and trace evidence.
package agentlaunch

import (
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// Resolution is the provider spawn specification plus trace-safe proof of how
// Matrix applied requested policy.
type Resolution struct {
	Endpoint middleware.ProtocolEndpoint
	Metadata map[string]interface{}
}

type policyAdapter interface {
	// Matches reports whether this adapter governs a launch. It decides on the
	// endpoint's own declaration, never on the agent's name.
	Matches(endpoint middleware.ProtocolEndpoint) bool
	Resolve(endpoint middleware.ProtocolEndpoint) (Resolution, error)
}

var policyAdapters = []policyAdapter{codexPolicyAdapter{}}

type codexPolicyAdapter struct{}

// Matches reports whether the codex launch-policy contract governs this launch.
//
// The endpoint decides by publishing the contract it implements
// (CodexPolicyContractEnv), which is the same declaration Resolve honors, so a
// provider that ships the contract is governed whatever its agent is called and
// a provider that does not ship it is never governed for being called codex.
//
// An endpoint that declares nothing is still this adapter's business when the
// launch asks for a guarantee only this contract provides, so that refusal stays
// loud instead of passing sandbox and approval keys through ungoverned.
func (codexPolicyAdapter) Matches(endpoint middleware.ProtocolEndpoint) bool {
	if envValue(endpoint.Env, CodexPolicyContractEnv) == CodexPolicyContractV1 {
		return true
	}
	state, err := readCodexPolicyState(endpoint)
	if err != nil {
		return true
	}
	return state.holdsGovernedPolicy()
}

// ResolveForAgent resolves an endpoint through the same contract used by run
// dispatch, doctor, and trace generation.
func ResolveForAgent(resolver middleware.AgentEndpointResolver, agentID string, launchArgs ...string) (Resolution, error) {
	if resolver == nil || strings.TrimSpace(agentID) == "" {
		return Resolution{}, nil
	}
	endpoint, err := resolver.GetAgentEndpoint(agentID)
	if err != nil {
		return Resolution{}, err
	}
	return ResolveEndpoint(agentID, endpoint, launchArgs...)
}

// ResolveEndpoint routes policy through a provider adapter. Agents without an
// adapter retain normal argv behavior.
//
// The agent is the one the caller resolved the endpoint for. It is retained
// because callers resolve by agent and the signature is their contract, and it
// is deliberately unused here: which policy governs a launch is the endpoint's
// declaration, not the name it was resolved under.
func ResolveEndpoint(_ string, endpoint middleware.ProtocolEndpoint, launchArgs ...string) (Resolution, error) {
	endpoint.Args = append(append([]string{}, endpoint.Args...), launchArgs...)
	endpoint.Env = append([]string{}, endpoint.Env...)
	for _, adapter := range policyAdapters {
		if adapter.Matches(endpoint) {
			return adapter.Resolve(endpoint)
		}
	}
	return Resolution{Endpoint: endpoint}, nil
}
