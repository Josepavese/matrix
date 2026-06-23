package agentlaunch

import (
	"errors"
	"strconv"
	"strings"
)

// CodexReasoningEffortArgs validates per-run Codex effort and returns launch args.
func CodexReasoningEffortArgs(agentID, generic, codex string) ([]string, error) {
	effort, err := resolveCodexReasoningEffort(agentID, generic, codex)
	if err != nil || effort == "" {
		return nil, err
	}
	return []string{"-c", "model_reasoning_effort=" + strconv.Quote(effort)}, nil
}

func resolveCodexReasoningEffort(agentID, generic, codex string) (string, error) {
	generic = strings.TrimSpace(generic)
	codex = strings.TrimSpace(codex)
	switch {
	case generic != "" && codex != "" && !strings.EqualFold(generic, codex):
		return "", errors.New("agent_config.model_reasoning_effort and codex_config.model_reasoning_effort disagree")
	case generic != "":
		return validateCodexReasoningEffort(agentID, generic)
	case codex != "":
		return validateCodexReasoningEffort(agentID, codex)
	default:
		return "", nil
	}
}

func validateCodexReasoningEffort(agentID, value string) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(agentID), "codex") {
		return "", errors.New("model_reasoning_effort is supported only when agent_id resolves to codex")
	}
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "low", "medium", "high", "xhigh":
		return value, nil
	default:
		return "", errors.New("unsupported model_reasoning_effort " + strconv.Quote(value) + " (supported: low, medium, high, xhigh)")
	}
}
