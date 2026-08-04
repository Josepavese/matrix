package agentlaunch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

const (
	// CodexPolicyContractEnv marks canonical Codex ACP installs supporting the
	// Matrix-owned environment translation contract.
	CodexPolicyContractEnv = "MATRIX_CODEX_LAUNCH_POLICY_CONTRACT"
	// CodexPolicyContractV1 uses CODEX_CONFIG and INITIAL_AGENT_MODE instead of
	// ignored wrapper argv.
	CodexPolicyContractV1 = "codex-acp-env-v1"

	codexConfigEnv      = "CODEX_CONFIG"
	codexInitialModeEnv = "INITIAL_AGENT_MODE"
)

type codexPolicyState struct {
	policy    map[string]string
	bypass    bool
	config    map[string]interface{}
	hasConfig bool
	mode      string
	requested map[string]interface{}
}

func (codexPolicyAdapter) Resolve(endpoint middleware.ProtocolEndpoint) (Resolution, error) {
	result := Resolution{Endpoint: endpoint}
	state, err := readCodexPolicyState(endpoint)
	if err != nil || !state.active() {
		return result, err
	}
	state.prepareRequested()
	if envValue(endpoint.Env, CodexPolicyContractEnv) != CodexPolicyContractV1 {
		return rejectCodexPolicy(result, state.requested, "missing_provider_contract",
			fmt.Errorf("codex launch policy is not applicable to this provider install; run `matrix install codex` with the current Matrix version"))
	}
	mode, status, err := resolveCodexMode(state.policy, state.mode)
	if err != nil {
		return rejectCodexPolicy(result, state.requested, status, err)
	}
	result.Endpoint, err = applyCodexPolicy(endpoint, state.policy, state.config, mode)
	if err != nil {
		return result, err
	}
	effective := recognizedConfig(state.config)
	applyModePolicy(effective, mode)
	state.defaultRequested(effective)
	result.Metadata = verifiedCodexMetadata(state, effective)
	return result, nil
}

func readCodexPolicyState(endpoint middleware.ProtocolEndpoint) (codexPolicyState, error) {
	policy, bypass := Parse(endpoint.Args)
	config, hasConfig, err := codexConfig(endpoint.Env)
	return codexPolicyState{
		policy: policy, bypass: bypass, config: config, hasConfig: hasConfig,
		mode: envValue(endpoint.Env, codexInitialModeEnv),
	}, err
}

func (s codexPolicyState) active() bool {
	return len(s.policy) > 0 || s.bypass || s.hasConfig || s.mode != ""
}

func (s *codexPolicyState) prepareRequested() {
	s.requested = stringMapToAny(s.policy)
	if s.bypass {
		s.requested["bypass_approvals_and_sandbox"] = true
		setDefault(s.policy, "sandbox_mode", "danger-full-access")
		setDefault(s.policy, "approval_policy", "never")
	}
}

func (s *codexPolicyState) defaultRequested(effective map[string]string) {
	if len(s.requested) == 0 {
		s.requested = stringMapToAny(effective)
	}
}

func resolveCodexMode(policy map[string]string, current string) (string, string, error) {
	desired, err := codexMode(policy)
	if err != nil {
		return "", "unsupported_policy", err
	}
	if desired != "" && current != "" && current != desired {
		return "", "conflicting_provider_mode", fmt.Errorf("codex launch policy conflicts with %s=%s", codexInitialModeEnv, current)
	}
	if desired != "" {
		current = desired
	}
	if current != "" && !validCodexMode(current) {
		return "", "unsupported_provider_mode", fmt.Errorf("unsupported %s value %q", codexInitialModeEnv, current)
	}
	return current, "", nil
}

func applyCodexPolicy(endpoint middleware.ProtocolEndpoint, policy map[string]string, config map[string]interface{}, mode string) (middleware.ProtocolEndpoint, error) {
	for key, value := range policy {
		config[key] = value
	}
	endpoint.Args = stripRecognizedPolicyArgs(endpoint.Args)
	if len(config) > 0 {
		encoded, err := json.Marshal(config)
		if err != nil {
			return endpoint, fmt.Errorf("encode %s: %w", codexConfigEnv, err)
		}
		endpoint.Env = upsertEnv(endpoint.Env, codexConfigEnv, string(encoded))
	}
	if mode != "" {
		endpoint.Env = upsertEnv(endpoint.Env, codexInitialModeEnv, mode)
	}
	return endpoint, nil
}

func verifiedCodexMetadata(state codexPolicyState, effective map[string]string) map[string]interface{} {
	meta := map[string]interface{}{
		"source":                policySource(state.policy, state.bypass),
		"requested":             state.requested,
		"effective":             stringMapToAny(effective),
		"application_mechanism": CodexPolicyContractV1,
		"verification_status":   "verified",
		"verified":              true,
	}
	if state.bypass {
		meta["bypass_approvals_and_sandbox"] = true
	}
	if effective["sandbox_mode"] == "danger-full-access" && effective["approval_policy"] == "never" {
		meta["trusted_terminal"] = true
	}
	return meta
}

// PreferredSessionMode returns exact provider mode Matrix must enforce after
// ACP session creation/resume. Empty means normal provider selection applies.
func PreferredSessionMode(endpoint middleware.ProtocolEndpoint) string {
	if envValue(endpoint.Env, CodexPolicyContractEnv) != CodexPolicyContractV1 {
		return ""
	}
	mode := envValue(endpoint.Env, codexInitialModeEnv)
	if validCodexMode(mode) {
		return mode
	}
	return ""
}

func codexConfig(env []string) (map[string]interface{}, bool, error) {
	raw := envValue(env, codexConfigEnv)
	if raw == "" {
		return map[string]interface{}{}, false, nil
	}
	var value map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, true, fmt.Errorf("invalid %s JSON object: %w", codexConfigEnv, err)
	}
	if value == nil {
		return nil, true, fmt.Errorf("invalid %s JSON object", codexConfigEnv)
	}
	return value, true, nil
}

func codexMode(policy map[string]string) (string, error) {
	sandbox, hasSandbox := policy["sandbox_mode"]
	approval, hasApproval := policy["approval_policy"]
	if !hasSandbox && !hasApproval {
		return "", nil
	}
	if !hasSandbox || !hasApproval {
		return "", fmt.Errorf("codex launch policy requires both sandbox_mode and approval_policy")
	}
	switch sandbox + "\x00" + approval {
	case "danger-full-access\x00never":
		return "agent-full-access", nil
	case "workspace-write\x00on-request":
		return "agent", nil
	case "read-only\x00on-request":
		return "read-only", nil
	default:
		return "", fmt.Errorf("codex launch policy sandbox_mode=%q approval_policy=%q has no canonical codex-acp mode", sandbox, approval)
	}
}

func validCodexMode(mode string) bool {
	return mode == "read-only" || mode == "agent" || mode == "agent-full-access"
}

func applyModePolicy(policy map[string]string, mode string) {
	values := map[string][2]string{
		"read-only":         {"read-only", "on-request"},
		"agent":             {"workspace-write", "on-request"},
		"agent-full-access": {"danger-full-access", "never"},
	}
	if value, ok := values[mode]; ok {
		policy["sandbox_mode"], policy["approval_policy"] = value[0], value[1]
	}
}

func recognizedConfig(config map[string]interface{}) map[string]string {
	out := map[string]string{}
	for key := range configKeys {
		if value, ok := config[key].(string); ok && strings.TrimSpace(value) != "" {
			out[key] = cleanValue(value)
		}
	}
	return out
}

func envValue(env []string, key string) string {
	prefix, value := key+"=", ""
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			value = strings.TrimPrefix(entry, prefix)
		}
	}
	return value
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}
	return append(out, prefix+value)
}

func rejectCodexPolicy(result Resolution, requested map[string]interface{}, status string, err error) (Resolution, error) {
	result.Metadata = map[string]interface{}{
		"requested":             requested,
		"application_mechanism": CodexPolicyContractV1,
		"verification_status":   status,
		"verified":              false,
	}
	return result, err
}

func stringMapToAny(values map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func policySource(policy map[string]string, bypass bool) string {
	if len(policy) > 0 || bypass {
		return "agent_args"
	}
	return "agent_env"
}

func setDefault(policy map[string]string, key, value string) {
	if policy[key] == "" {
		policy[key] = value
	}
}
