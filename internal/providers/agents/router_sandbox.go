package agents

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

func (c *acpConversationClient) verifySandboxSessionCwd(reported, requested string) error {
	if c.containerSandbox() {
		if reported == "" || reported == requested {
			return nil
		}
		return fmt.Errorf("provider reported another container workspace")
	}
	return verifyListedSessionCwd(reported, requested)
}

// A changed policy cannot reuse a child that was launched under another one.
// Refuse until its existing leases finish and the client is reaped; do not
// terminate an unrelated sibling prompt or open the same provider state twice.
func (r *Router) cachedLaunchMatches(key string, client middleware.ConversationClient) (bool, error) {
	acp, ok := client.(*acpConversationClient)
	if !ok || r.resolver == nil {
		return true, nil
	}
	agentID, _, args := splitClientCacheKeyParts(key)
	endpoint, err := r.resolver.GetAgentEndpoint(agentID)
	if err != nil {
		return false, err
	}
	resolved, err := agentlaunch.ResolveEndpoint(agentID, endpoint, args...)
	if err != nil {
		return false, err
	}
	// Existing endpoints without sandbox preserve the historical cache contract.
	if acp.endpoint.Sandbox == nil && resolved.Endpoint.Sandbox == nil {
		return true, nil
	}
	first, err := json.Marshal(acp.endpoint)
	if err != nil {
		return false, fmt.Errorf("cached launch declaration invalid")
	}
	second, err := json.Marshal(resolved.Endpoint)
	if err != nil {
		return false, fmt.Errorf("current launch declaration invalid")
	}
	return reflect.DeepEqual(first, second), nil
}

func (r *Router) lookupAnyReusableClientForAgent(agentID string) (middleware.ConversationClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for key, client := range r.clients {
		candidateAgentID, _ := splitClientCacheKey(key)
		if candidateAgentID == agentID && isReusableClient(client) {
			if matches, err := r.cachedLaunchMatches(key, client); err != nil || !matches {
				continue
			}
			return client, true
		}
	}
	return nil, false
}
