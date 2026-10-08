package agentlaunch

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestNativeSandboxPreservesRuleOrderAndAddsOnlyDenials(t *testing.T) {
	for contract, key := range nativePermissionEnvironment {
		endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Env: []string{
			SandboxEnv + `={"native_contract":"` + contract + `","native_profile":"read-only"}`,
			key + `={"*":"ask","read":{"*":"deny","public/*":"allow"},"bash":{"rm *":"deny","echo *":"allow"}}`,
		}}
		resolved, err := ResolveEndpoint("arbitrary-provider-name", endpoint)
		if err != nil {
			t.Fatal(err)
		}
		raw := envValue(resolved.Endpoint.Env, key)
		if !strings.HasPrefix(raw, `{"*":"ask","read":{"*":"deny","public/*":"allow"}`) {
			t.Fatal("existing rule order changed", raw)
		}
		var rules map[string]interface{}
		if json.Unmarshal([]byte(raw), &rules) != nil || rules["bash"] != "deny" || rules["edit"] != "deny" {
			t.Fatal("required denials missing", raw)
		}
		again, err := ResolveEndpoint("other-name", resolved.Endpoint)
		if err != nil || envValue(again.Endpoint.Env, key) != raw {
			t.Fatal("policy not idempotent", err)
		}
		meta, ok := resolved.Metadata["sandbox"].(map[string]interface{})
		if !ok || meta["native_status"] != "configured_not_attested" {
			t.Fatal("configuration claimed enforcement")
		}
	}
}

func TestSandboxRejectsMalformedUnsupportedRemoteAndBypass(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"native_contract":"opencode-permission-v1","native_profile":"read-only","native_profile":"workspace"}`, `{"unknown":1}`, `{"native_contract":"unknown","native_profile":"read-only"}`, `{"native_contract":"opencode-permission-v1","native_profile":"full-access"}`, `{"native_contract":"opencode-permission-v1","native_profile":"read-only"} {}`} {
		if _, err := ResolveEndpoint("opencode", middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Env: []string{SandboxEnv + "=" + raw}}); err == nil {
			t.Fatal("invalid request accepted", raw)
		}
	}
	endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "ws", Env: []string{SandboxEnv + `={"native_contract":"mimocode-permission-v1","native_profile":"workspace"}`}}
	if _, err := ResolveEndpoint("remote", endpoint); err == nil {
		t.Fatal("remote enforcement inferred")
	}
	endpoint.Transport = "stdio"
	endpoint.Env = append(endpoint.Env, "MIMOCODE_DANGEROUSLY_SKIP_PERMISSIONS=1")
	if _, err := ResolveEndpoint("mimo", endpoint); err == nil {
		t.Fatal("bypass silently accepted")
	}
	for _, permission := range []string{`{"read":"deny","read":"allow"}`, `{"bash":123}`, `{"*":"unexpected"}`} {
		if _, err := appendPermissionDenials(permission, []string{"edit"}); err == nil {
			t.Fatal("invalid permissions accepted", permission)
		}
	}
}
