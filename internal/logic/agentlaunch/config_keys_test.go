package agentlaunch

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestReasoningEffortFollowsTheProviderDeclaration is the agnosticism contract
// for per-run reasoning effort: the setting is accepted because the endpoint
// declares the key, not because of whose endpoint it is.
//
// Every case asks about "mimo", a name no branch in this package knows. The
// agent whose endpoint declares the key is accepted and gets launch args; the
// agent whose endpoint does not is refused. Same name, same request, different
// declaration: a name-based decision cannot answer both.
func TestReasoningEffortFollowsTheProviderDeclaration(t *testing.T) {
	declaring := middleware.ProtocolEndpoint{
		Env: []string{ConfigKeysEnv + "=" + ConfigKeyList(ModelReasoningEffortKey)},
	}
	silent := middleware.ProtocolEndpoint{Env: []string{"UNRELATED=1"}}

	for _, test := range []struct {
		why      string
		endpoint middleware.ProtocolEndpoint
		value    string
		wantArgs []string
		wantErr  string
	}{
		{
			why:      "endpoint declares the key",
			endpoint: declaring,
			value:    "xhigh",
			wantArgs: []string{"-c", `model_reasoning_effort="xhigh"`},
		},
		{
			why:      "endpoint declares the key, value out of range",
			endpoint: declaring,
			value:    "turbo",
			wantErr:  "unsupported model_reasoning_effort",
		},
		{
			why:      "endpoint declares nothing",
			endpoint: silent,
			value:    "xhigh",
			wantErr:  "requires a provider that declares",
		},
		{
			why:      "no endpoint at all",
			endpoint: middleware.ProtocolEndpoint{},
			value:    "xhigh",
			wantErr:  "requires a provider that declares",
		},
	} {
		t.Run(test.why, func(t *testing.T) {
			const agentID = "mimo"
			resolver := staticEndpointResolver{endpoint: test.endpoint}
			declared := DeclaredConfigKeysForAgent(resolver, agentID)

			args, err := ReasoningEffortArgs(declared, test.value, "")
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("ReasoningEffortArgs(%q) error = %v, want it to mention %q", test.value, err, test.wantErr)
				}
				if args != nil {
					t.Fatalf("refused setting still produced launch args: %#v", args)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReasoningEffortArgs(%q): %v", test.value, err)
			}
			if strings.Join(args, "\x00") != strings.Join(test.wantArgs, "\x00") {
				t.Fatalf("launch args = %#v, want %#v", args, test.wantArgs)
			}
		})
	}
}

// TestReasoningEffortAcceptsEitherNamespaceWhenDeclared pins the other half of
// the decision: once the provider declares the key, the namespace the caller
// typed it in is not a reason to accept one spelling and refuse the other.
func TestReasoningEffortAcceptsEitherNamespaceWhenDeclared(t *testing.T) {
	declared := DeclaredConfigKeys(middleware.ProtocolEndpoint{
		Env: []string{ConfigKeysEnv + "=" + ConfigKeyList(ModelReasoningEffortKey)},
	})

	fromGeneric, err := ReasoningEffortArgs(declared, "high", "")
	if err != nil {
		t.Fatalf("agent_config namespace: %v", err)
	}
	fromVendor, err := ReasoningEffortArgs(declared, "", "high")
	if err != nil {
		t.Fatalf("codex_config namespace: %v", err)
	}
	if strings.Join(fromGeneric, "\x00") != strings.Join(fromVendor, "\x00") {
		t.Fatalf("the same declared key produced different args: %#v vs %#v", fromGeneric, fromVendor)
	}
	if _, err := ReasoningEffortArgs(declared, "high", "low"); err == nil {
		t.Fatal("disagreeing namespaces should still be refused")
	}
}

// TestDeclaredConfigKeysReadsOnlyTheEndpointDeclaration keeps the declaration
// itself honest: it is data on the endpoint, normalized for comparison, and
// nothing else in the environment contributes a key.
func TestDeclaredConfigKeysReadsOnlyTheEndpointDeclaration(t *testing.T) {
	declared := DeclaredConfigKeys(middleware.ProtocolEndpoint{
		Env: []string{
			ConfigKeysEnv + " = model_reasoning_effort , sandbox_mode ,, model_reasoning_effort",
			"MODEL_REASONING_EFFORT=smuggled",
		},
	})
	if len(declared) != 2 || declared[ModelReasoningEffortKey] == "" || declared["sandbox_mode"] == "" {
		t.Fatalf("declared keys = %#v, want exactly model_reasoning_effort and sandbox_mode", declared)
	}
	if len(DeclaredConfigKeys(middleware.ProtocolEndpoint{})) != 0 {
		t.Fatal("an endpoint that declares nothing must declare an empty set")
	}
}

type staticEndpointResolver struct {
	endpoint middleware.ProtocolEndpoint
}

func (r staticEndpointResolver) GetAgentEndpoint(string) (middleware.ProtocolEndpoint, error) {
	return r.endpoint, nil
}
