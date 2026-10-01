package agentlaunch

import (
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// ConfigKeysEnv is the declarative launch contract: the launch configuration
// keys an endpoint accepts, published by the provider that implements them.
//
// It exists so a per-run setting is validated against what the agent declares
// it takes, instead of against the agent's name. A provider that supports one
// of the keys adds it to this declaration at install time; Matrix then accepts
// that key for any agent whose endpoint carries it, and for no other. The
// declaration is a property of the install, not a table of identities: nothing
// here needs to change when an agent is added.
const ConfigKeysEnv = "MATRIX_LAUNCH_CONFIG_KEYS"

// ModelReasoningEffortKey is the launch configuration key that carries per-run
// reasoning effort. The provider contract that implements it declares it.
const ModelReasoningEffortKey = "model_reasoning_effort"

// DeclaredConfigKeys returns the launch configuration keys an endpoint accepts.
//
// An endpoint that declares nothing returns an empty set, and an empty set is
// not a licence: a caller that supplies a setting refuses rather than passing
// it to a provider that never claimed to read it.
func DeclaredConfigKeys(endpoint middleware.ProtocolEndpoint) map[string]string {
	keys := map[string]string{}
	for _, key := range strings.Split(declaredConfigKeysValue(endpoint.Env), ",") {
		if key = normalizeConfigKey(key); key != "" {
			keys[key] = key
		}
	}
	return keys
}

// declaredConfigKeysValue reads the contract entry. The key is matched exactly:
// a differently spelled variable that happens to end in the same words is not
// the provider speaking, and the value is what carries the declaration.
func declaredConfigKeysValue(env []string) string {
	value := ""
	for _, entry := range env {
		name, entryValue, ok := strings.Cut(entry, "=")
		if ok && strings.TrimSpace(name) == ConfigKeysEnv {
			value = entryValue
		}
	}
	return value
}

// DeclaredConfigKeysForAgent resolves an agent's endpoint and reads the launch
// configuration keys it declares. It is the entry point for callers that hold a
// resolver rather than an endpoint; a nil resolver, an unknown agent, or a
// resolver failure declares nothing.
func DeclaredConfigKeysForAgent(resolver middleware.AgentEndpointResolver, agentID string) map[string]string {
	if resolver == nil || strings.TrimSpace(agentID) == "" {
		return map[string]string{}
	}
	endpoint, err := resolver.GetAgentEndpoint(agentID)
	if err != nil {
		return map[string]string{}
	}
	return DeclaredConfigKeys(endpoint)
}

// normalizeConfigKey reduces a declared key to the spelling both sides compare
// with: trimmed and lower case, because a declaration is configuration data and
// its casing is not a claim about behaviour.
func normalizeConfigKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// ConfigKeyList renders declared keys for a contract value, so the declaration
// a provider publishes and the declaration Matrix reads cannot drift in
// spelling or order.
func ConfigKeyList(keys ...string) string {
	unique := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, key := range keys {
		key = normalizeConfigKey(key)
		if key != "" && !seen[key] {
			seen[key] = true
			unique = append(unique, key)
		}
	}
	return strings.Join(unique, ",")
}
