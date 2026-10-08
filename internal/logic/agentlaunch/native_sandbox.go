package agentlaunch

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

var nativePermissionEnvironment = map[string]string{
	"opencode-permission-v1": "OPENCODE_PERMISSION",
	"mimocode-permission-v1": "MIMOCODE_PERMISSION",
}

func applyNativeSandbox(endpoint middleware.ProtocolEndpoint, policy middleware.SandboxPolicy) (middleware.ProtocolEndpoint, error) {
	if envValue(endpoint.Env, CodexPolicyContractEnv) != "" {
		return endpoint, fmt.Errorf("conflicting native provider contracts")
	}
	for _, entry := range endpoint.Env {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasSuffix(key, "DANGEROUSLY_SKIP_PERMISSIONS") && value != "" && value != "0" {
			return endpoint, fmt.Errorf("native sandbox conflicts with permission bypass")
		}
	}
	for _, arg := range endpoint.Args {
		if strings.HasPrefix(arg, "--dangerously") {
			return endpoint, fmt.Errorf("native sandbox conflicts with permission bypass")
		}
	}
	key := nativePermissionEnvironment[policy.NativeContract]
	denials := []string{"external_directory", "task", "actor"}
	if policy.NativeProfile == "read-only" {
		denials = append(denials, "edit", "write", "patch", "multiedit", "bash")
	}
	merged, err := appendPermissionDenials(envValue(endpoint.Env, key), denials)
	if err != nil {
		return endpoint, err
	}
	endpoint.Env = upsertEnv(endpoint.Env, key, merged)
	return endpoint, nil
}

type permissionEntry struct {
	key   string
	value json.RawMessage
}

// Keep existing key order and nested rules verbatim. Provider precedence is last
// match; sorting patterns would change policy. Only append restrictive denials.
func appendPermissionDenials(raw string, denials []string) (string, error) {
	entries, err := permissionEntries(raw)
	if err != nil {
		return "", err
	}
	deny := map[string]bool{}
	for _, key := range denials {
		deny[key] = true
	}
	parts := make([]string, 0, len(entries)+len(denials))
	for _, entry := range entries {
		if !deny[entry.key] {
			key, _ := json.Marshal(entry.key)
			parts = append(parts, string(key)+":"+string(entry.value))
		}
	}
	for _, key := range denials {
		parts = append(parts, `"`+key+`":"deny"`)
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

func permissionEntries(raw string) ([]permissionEntry, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 16384 {
		return nil, fmt.Errorf("native permission declaration exceeds limit")
	}
	if permissionScalar(raw) {
		return []permissionEntry{{"*", json.RawMessage(raw)}}, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("invalid native permission object")
	}
	entries := []permissionEntry{}
	seen := map[string]bool{}
	for decoder.More() {
		entry, err := decodePermissionEntry(decoder, seen)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("invalid native permission object")
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		return nil, fmt.Errorf("invalid trailing permission data")
	}
	return entries, nil
}

func validPermissionRule(value json.RawMessage) bool {
	var action string
	if json.Unmarshal(value, &action) == nil {
		return validPermissionAction(action)
	}
	var patterns map[string]string
	if json.Unmarshal(value, &patterns) != nil || patterns == nil {
		return false
	}
	for _, action := range patterns {
		if !validPermissionAction(action) {
			return false
		}
	}
	return true
}

func validPermissionAction(value string) bool {
	return value == "deny" || value == "ask" || value == "allow"
}

func decodePermissionEntry(decoder *json.Decoder, seen map[string]bool) (permissionEntry, error) {
	token, err := decoder.Token()
	if err != nil {
		return permissionEntry{}, fmt.Errorf("invalid native permission key")
	}
	key, ok := token.(string)
	if !ok || seen[key] {
		return permissionEntry{}, fmt.Errorf("duplicate native permission key")
	}
	seen[key] = true
	var value json.RawMessage
	if err := decoder.Decode(&value); err != nil || !validPermissionRule(value) {
		return permissionEntry{}, fmt.Errorf("invalid native permission rule")
	}
	return permissionEntry{key, value}, nil
}

func permissionScalar(raw string) bool {
	var action string
	return json.Unmarshal([]byte(raw), &action) == nil && validPermissionAction(action)
}
