package agentlaunch

import "strings"

const bypassFlag = "--dangerously-bypass-approvals-and-sandbox"

var configKeys = map[string]string{
	"approval_policy":        "approval_policy",
	"model_reasoning_effort": "model_reasoning_effort",
	"sandbox_mode":           "sandbox_mode",
	"sandbox_permissions":    "sandbox_permissions",
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

func stripRecognizedPolicyArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == bypassFlag {
			continue
		}
		if key, ok := nextArgKeys[arg]; ok && i+1 < len(args) && removablePolicyValue(key, args[i+1]) {
			i++
			continue
		}
		if key, value, ok := prefixedValue(arg); ok && removablePolicyValue(key, value) {
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func removablePolicyValue(key, value string) bool {
	return key != "config" || recognizedConfigAssignment(value)
}

func recognizedConfigAssignment(value string) bool {
	key, _, ok := strings.Cut(value, "=")
	return ok && configKeys[strings.TrimSpace(key)] != ""
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

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	return strings.Trim(value, `'`)
}
