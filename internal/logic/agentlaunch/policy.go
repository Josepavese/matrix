// Package agentlaunch resolves governed provider launch policy and trace evidence.
package agentlaunch

import (
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentidentity"
	"github.com/Josepavese/matrix/internal/middleware"
)

// Resolution is the provider spawn specification plus trace-safe proof of how
// Matrix applied requested policy.
type Resolution struct {
	Endpoint middleware.ProtocolEndpoint
	Metadata map[string]interface{}
}

type policyAdapter interface {
	Matches(agentID string, endpoint middleware.ProtocolEndpoint) bool
	Resolve(endpoint middleware.ProtocolEndpoint) (Resolution, error)
}

var policyAdapters = []policyAdapter{codexPolicyAdapter{}}

type codexPolicyAdapter struct{}

func (codexPolicyAdapter) Matches(agentID string, _ middleware.ProtocolEndpoint) bool {
	return strings.EqualFold(strings.TrimSpace(agentID), agentidentity.CanonicalCodexAgentID)
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
func ResolveEndpoint(agentID string, endpoint middleware.ProtocolEndpoint, launchArgs ...string) (Resolution, error) {
	endpoint.Args = append(append([]string{}, endpoint.Args...), launchArgs...)
	endpoint.Env = append([]string{}, endpoint.Env...)
	for _, adapter := range policyAdapters {
		if adapter.Matches(agentID, endpoint) {
			return adapter.Resolve(endpoint)
		}
	}
	return Resolution{Endpoint: endpoint}, nil
}
