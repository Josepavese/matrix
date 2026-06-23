// Package agentlaunch extracts safe audit metadata from agent launch arguments.
package agentlaunch

import (
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

const bypassFlag = "--dangerously-bypass-approvals-and-sandbox"

var configKeys = map[string]string{
	"approval_policy":     "approval_policy",
	"sandbox_mode":        "sandbox_mode",
	"sandbox_permissions": "sandbox_permissions",
}

var nextArgKeys = map[string]string{
	"-a":                 "approval_policy",
	"--ask-for-approval": "approval_policy",
	"-c":                 "config",
	"--config":           "config",
	"-s":                 "sandbox_mode",
	"--sandbox":          "sandbox_mode",
}

var prefixedArgKeys = []argPrefix{
	{prefix: "--ask-for-approval=", key: "approval_policy"},
	{prefix: "--config=", key: "config"},
	{prefix: "--sandbox=", key: "sandbox_mode"},
	{prefix: "-c=", key: "config"},
}

type argPrefix struct {
	prefix string
	key    string
}

// MetadataForAgent resolves an agent endpoint and returns trace-safe launch policy.
func MetadataForAgent(resolver middleware.AgentEndpointResolver, agentID string) map[string]interface{} {
	if resolver == nil || strings.TrimSpace(agentID) == "" {
		return nil
	}
	endpoint, err := resolver.GetAgentEndpoint(agentID)
	if err != nil {
		return nil
	}
	return Metadata(endpoint.Args)
}

// Metadata returns trace-safe launch policy details inferred from argv.
func Metadata(args []string) map[string]interface{} {
	policy, bypass := Parse(args)
	if len(policy) == 0 && !bypass {
		return nil
	}
	if bypass {
		setDefault(policy, "sandbox_mode", "danger-full-access")
		setDefault(policy, "approval_policy", "never")
	}
	meta := map[string]interface{}{"source": "agent_args"}
	addValue(meta, policy, "sandbox_mode")
	addValue(meta, policy, "approval_policy")
	addValue(meta, policy, "sandbox_permissions")
	if bypass {
		meta["bypass_approvals_and_sandbox"] = true
	}
	if policy["sandbox_mode"] == "danger-full-access" && policy["approval_policy"] == "never" {
		meta["trusted_terminal"] = true
	}
	return meta
}

// Parse extracts recognized launch policy keys from agent argv.
func Parse(args []string) (map[string]string, bool) {
	policy := map[string]string{}
	bypass := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == bypassFlag {
			bypass = true
			continue
		}
		if key, ok := nextArgKeys[arg]; ok {
			i = collectNextArgValue(policy, args, i, key)
			continue
		}
		if key, value, ok := prefixedValue(arg); ok {
			collectArgValue(policy, key, value)
		}
	}
	return policy, bypass
}

func collectNextArgValue(policy map[string]string, args []string, index int, key string) int {
	if index+1 >= len(args) {
		return index
	}
	collectArgValue(policy, key, args[index+1])
	return index + 1
}

func prefixedValue(arg string) (string, string, bool) {
	for _, candidate := range prefixedArgKeys {
		if strings.HasPrefix(arg, candidate.prefix) {
			return candidate.key, strings.TrimPrefix(arg, candidate.prefix), true
		}
	}
	return "", "", false
}

func collectArgValue(policy map[string]string, key, value string) {
	if key == "config" {
		collectConfigValue(policy, value)
		return
	}
	policy[key] = cleanValue(value)
}

func collectConfigValue(policy map[string]string, config string) {
	key, value, ok := strings.Cut(config, "=")
	if !ok {
		return
	}
	if policyKey := configKeys[strings.TrimSpace(key)]; policyKey != "" {
		policy[policyKey] = cleanValue(value)
	}
}

func addValue(meta map[string]interface{}, policy map[string]string, key string) {
	if value := policy[key]; value != "" {
		meta[key] = value
	}
}

func setDefault(policy map[string]string, key, value string) {
	if policy[key] == "" {
		policy[key] = value
	}
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	return strings.Trim(value, `'`)
}
