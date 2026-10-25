package agentlaunch

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func acpEndpointWithMode(mode string) middleware.ProtocolEndpoint {
	return middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindACP,
		Env:  []string{ACPSessionModeEnv + "=" + mode},
	}
}

func sessionModeMetadataOf(t *testing.T, resolved Resolution) map[string]interface{} {
	t.Helper()
	raw, ok := resolved.Metadata["session_mode"]
	if !ok {
		t.Fatalf("launch evidence carries no session_mode: %#v", resolved.Metadata)
	}
	contract, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("session_mode evidence is not a contract map: %#v", raw)
	}
	return contract
}

// TestResolveEndpointRecordsRequiredSessionModeContract pins the launch half of
// the neutral contract: the declaration is validated, kept on the endpoint, and
// recorded as a required contract whose verification is the provider session's
// to give, never the launch resolution's.
func TestResolveEndpointRecordsRequiredSessionModeContract(t *testing.T) {
	resolved, err := ResolveEndpoint("opencode", acpEndpointWithMode("json-planning"))
	if err != nil {
		t.Fatal(err)
	}
	// The resolved endpoint keeps the declaration because the ACP adapter reads
	// the same contract when it applies and verifies the mode. It is inert to
	// the provider, which reads only its own configuration.
	if envValues(resolved.Endpoint.Env, ACPSessionModeEnv)[0] != "json-planning" {
		t.Fatalf("the resolved endpoint lost the declaration: %#v", resolved.Endpoint.Env)
	}
	contract := sessionModeMetadataOf(t, resolved)
	if contract["configured"] != "json-planning" {
		t.Fatalf("configured mode lost: %#v", contract)
	}
	if contract["enforcement"] != sessionModeEnforcementRequired {
		t.Fatalf("a declared mode must be recorded as required, got %#v", contract)
	}
	if contract["verification_status"] != sessionModeVerificationPending || contract["verified"] != false {
		t.Fatalf("launch resolution must not claim provider application: %#v", contract)
	}
	if contract["application_mechanism"] != sessionModeApplicationMechanism {
		t.Fatalf("unexpected application mechanism: %#v", contract)
	}
	if got := RequiredSessionMode(resolved.Endpoint); got != "json-planning" {
		t.Fatalf("RequiredSessionMode=%q", got)
	}
}

func TestRequiredSessionModeIsEmptyWithoutTheContract(t *testing.T) {
	if got := RequiredSessionMode(middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP}); got != "" {
		t.Fatalf("an endpoint that declares no mode requires none, got %q", got)
	}
}

// TestResolveEndpointRefusesMalformedSessionMode keeps the contract strict: a
// value that is not a mode identifier is refused before a provider is launched,
// instead of being normalized into something a provider might accept.
func TestResolveEndpointRefusesMalformedSessionMode(t *testing.T) {
	long := strings.Repeat("a", 65)
	for _, value := range []string{"", "json mode", "json/planning", "-json", "json\n", long} {
		t.Run("value "+value, func(t *testing.T) {
			resolved, err := ResolveEndpoint("opencode", acpEndpointWithMode(value))
			if err == nil {
				t.Fatalf("expected a malformed %s=%q to be refused", ACPSessionModeEnv, value)
			}
			if !strings.Contains(err.Error(), ACPSessionModeEnv) {
				t.Fatalf("refusal does not name the contract: %v", err)
			}
			if got := sessionModeMetadataOf(t, resolved)["verification_status"]; got != sessionModeRejected {
				t.Fatalf("refused contract reported as %q", got)
			}
		})
	}
}

func TestResolveEndpointRefusesDuplicateSessionModeDeclarations(t *testing.T) {
	for _, env := range [][]string{
		{ACPSessionModeEnv + "=json", ACPSessionModeEnv + "=json"},
		{ACPSessionModeEnv + "=json", ACPSessionModeEnv + "=plan"},
	} {
		_, err := ResolveEndpoint("opencode", middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Env: env})
		if err == nil || !strings.Contains(err.Error(), "declared once") {
			t.Fatalf("duplicate declaration not refused: %v", err)
		}
	}
}

func TestResolveEndpointRefusesSessionModeOnNonACPEnvelope(t *testing.T) {
	_, err := ResolveEndpoint("remote", middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindA2A,
		Env:  []string{ACPSessionModeEnv + "=json"},
	})
	if err == nil || !strings.Contains(err.Error(), "ACP endpoints only") {
		t.Fatalf("expected ACP-only refusal, got %v", err)
	}
}

// TestResolveEndpointRefusesConflictingSessionModeContracts keeps one owner of
// the session mode: a launch that declares both the neutral contract and the
// Codex launch policy is refused, whatever form the Codex declaration took.
func TestResolveEndpointRefusesConflictingSessionModeContracts(t *testing.T) {
	cases := map[string]middleware.ProtocolEndpoint{
		"initial mode env": {
			Kind: middleware.ProtocolKindACP,
			Env: []string{
				CodexPolicyContractEnv + "=" + CodexPolicyContractV1,
				codexInitialModeEnv + "=agent-full-access",
				ACPSessionModeEnv + "=json-planning",
			},
		},
		"policy argv": {
			Kind: middleware.ProtocolKindACP,
			Args: []string{"wrapper.js", "-c", "sandbox_mode=danger-full-access", "-c", "approval_policy=never"},
			Env: []string{
				CodexPolicyContractEnv + "=" + CodexPolicyContractV1,
				ACPSessionModeEnv + "=json-planning",
			},
		},
		"codex config": {
			Kind: middleware.ProtocolKindACP,
			Env: []string{
				codexConfigEnv + `={"sandbox_mode":"danger-full-access","approval_policy":"never"}`,
				ACPSessionModeEnv + "=json-planning",
			},
		},
	}
	for name, endpoint := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ResolveEndpoint("codex", endpoint)
			if err == nil || !strings.Contains(err.Error(), "conflicts with the Codex launch-policy contract") {
				t.Fatalf("expected a two-contract conflict, got %v", err)
			}
			if !strings.Contains(err.Error(), ACPSessionModeEnv) {
				t.Fatalf("conflict refusal does not name the neutral contract: %v", err)
			}
		})
	}
}

// TestResolveEndpointRefusesConflictingModeNames keeps the conflict message
// actionable: when both contracts own a named mode, both names are reported.
func TestResolveEndpointRefusesConflictingModeNames(t *testing.T) {
	_, err := ResolveEndpoint("codex", middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindACP,
		Env: []string{
			CodexPolicyContractEnv + "=" + CodexPolicyContractV1,
			codexInitialModeEnv + "=read-only",
			ACPSessionModeEnv + "=json-planning",
		},
	})
	if err == nil {
		t.Fatal("expected a conflict")
	}
	for _, value := range []string{`"json-planning"`, `"read-only"`} {
		if !strings.Contains(err.Error(), value) {
			t.Fatalf("conflict message does not name %s: %v", value, err)
		}
	}
}

// TestCodexOnlyLaunchKeepsItsContract is the regression half: adding the
// neutral adapter must not change a launch that only declares the Codex
// contract.
func TestCodexOnlyLaunchKeepsItsContract(t *testing.T) {
	resolved, err := ResolveEndpoint("codex", middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindACP,
		Env: []string{
			CodexPolicyContractEnv + "=" + CodexPolicyContractV1,
			codexInitialModeEnv + "=agent",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Metadata["application_mechanism"] != CodexPolicyContractV1 {
		t.Fatalf("codex launch evidence changed: %#v", resolved.Metadata)
	}
	if _, ok := resolved.Metadata["session_mode"]; ok {
		t.Fatalf("a codex-only launch must not grow the neutral session mode evidence: %#v", resolved.Metadata)
	}
	if got := PreferredSessionMode(resolved.Endpoint); got != "agent" {
		t.Fatalf("codex preferred mode=%q", got)
	}
	if got := RequiredSessionMode(resolved.Endpoint); got != "" {
		t.Fatalf("codex-only launch must not require the neutral mode, got %q", got)
	}
}
