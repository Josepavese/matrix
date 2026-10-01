package agentlaunch

import (
	"errors"
	"strconv"
	"strings"
)

// ConfigArgPrefix is the launch argument form a declared configuration key
// travels in: the CLI spelling Matrix translates a declared key into.
const ConfigArgPrefix = "-c"

// ReasoningEffortArgs validates per-run reasoning effort against the launch
// configuration keys the agent's endpoint declares, and returns launch args.
//
// A value is accepted when the endpoint declares the key, whichever config
// namespace supplied it: the key is the same setting either way, and refusing
// one spelling while accepting the other would decide on the caller's choice of
// field name rather than on the provider's declaration. With no declaration
// there is no contract to satisfy, and the caller is told so instead of having
// its setting silently dropped or its agent judged by name.
func ReasoningEffortArgs(declared map[string]string, generic, vendor string) ([]string, error) {
	effort, err := resolveReasoningEffort(declared, generic, vendor)
	if err != nil || effort == "" {
		return nil, err
	}
	return []string{ConfigArgPrefix, ModelReasoningEffortKey + "=" + strconv.Quote(effort)}, nil
}

func resolveReasoningEffort(declared map[string]string, generic, vendor string) (string, error) {
	generic = strings.TrimSpace(generic)
	vendor = strings.TrimSpace(vendor)
	switch {
	case generic != "" && vendor != "" && !strings.EqualFold(generic, vendor):
		return "", errors.New("agent_config.model_reasoning_effort and codex_config.model_reasoning_effort disagree")
	case generic != "":
		return validateReasoningEffort(declared, generic)
	case vendor != "":
		return validateReasoningEffort(declared, vendor)
	default:
		return "", nil
	}
}

func validateReasoningEffort(declared map[string]string, value string) (string, error) {
	if declared[ModelReasoningEffortKey] == "" {
		return "", errors.New(ModelReasoningEffortKey + " requires a provider that declares " + ConfigKeysEnv + "=" + ModelReasoningEffortKey)
	}
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "low", "medium", "high", "xhigh":
		return value, nil
	default:
		return "", errors.New("unsupported model_reasoning_effort " + strconv.Quote(value) + " (supported: low, medium, high, xhigh)")
	}
}
