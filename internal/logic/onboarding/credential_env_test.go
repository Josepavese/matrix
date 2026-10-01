package onboarding

import (
	"context"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
)

// declaredMethodHandler is an agent whose published auth methods are the only
// thing that names its credential variable. It knows nothing about agent names,
// so it can answer for any agent the test asks about.
type declaredMethodHandler struct {
	methods []AuthMethod
}

func (h declaredMethodHandler) Methods(context.Context) ([]AuthMethod, error) {
	return h.methods, nil
}

func (h declaredMethodHandler) Authenticate(context.Context, AuthMethod, string) (*AuthResult, string, error) {
	return &AuthResult{}, "", nil
}

func (h declaredMethodHandler) Name() string { return "declared-method" }

// TestCredentialEnvNameFollowsThePublishedMethod is the agnosticism contract for
// credential naming: the variable a typed key is stored under is the one the
// agent's auth method declares.
func TestCredentialEnvNameFollowsThePublishedMethod(t *testing.T) {
	registry := &authHandlerRegistry{
		handlers: map[string]AuthHandler{
			// A name no branch in this package knows: the answer must not need it.
			"kimi": declaredMethodHandler{methods: []AuthMethod{
				{ID: "device", Name: "Device login", Type: "agent"},
				{ID: "moonshot-key", Name: "Moonshot key", Type: "env_var", Vars: []string{"MOONSHOT_API_KEY"}},
			}},
		},
		fallback: &acpAuthHandler{},
	}

	if got := registry.credentialEnvName("kimi", "moonshot-key"); got != "MOONSHOT_API_KEY" {
		t.Fatalf("declared method name = %q, want MOONSHOT_API_KEY", got)
	}
	if got := registry.credentialEnvName("kimi", ""); got != "MOONSHOT_API_KEY" {
		t.Fatalf("with no recorded method the env_var method should answer, got %q", got)
	}
	if got := registry.credentialEnvName("kimi", "device"); got != "MOONSHOT_API_KEY" {
		t.Fatalf("a method declaring no variable leaves the key to the env_var method, got %q", got)
	}

	silent := &authHandlerRegistry{
		handlers: map[string]AuthHandler{
			"kimi": declaredMethodHandler{methods: []AuthMethod{{ID: "device", Type: "agent"}}},
		},
		fallback: &acpAuthHandler{},
	}
	if got := silent.credentialEnvName("kimi", "device"); got != "" {
		t.Fatalf("an agent declaring no credential variable must yield no name, got %q", got)
	}
}

// TestConfigureAgentStoresKeyUnderTheDeclaredVariable drives the wizard path
// that used to decide the variable from the agent's name: the same agent name is
// configured twice, and only the declaration changes between the runs.
func TestConfigureAgentStoresKeyUnderTheDeclaredVariable(t *testing.T) {
	for _, test := range []struct {
		why      string
		methods  []AuthMethod
		wantEnv  string
		notInEnv string
	}{
		{
			why:      "agent declares its credential variable",
			methods:  []AuthMethod{{ID: "api-key", Type: "env_var", Vars: []string{"MOONSHOT_API_KEY"}}},
			wantEnv:  "MOONSHOT_API_KEY=typed-key",
			notInEnv: "OPENAI_API_KEY=",
		},
		{
			why:      "agent declares no credential variable",
			methods:  []AuthMethod{{ID: "device", Type: "agent"}},
			wantEnv:  "API_KEY=typed-key",
			notInEnv: "OPENAI_API_KEY=",
		},
	} {
		t.Run(test.why, func(t *testing.T) {
			w := newTestWizard()
			w.handlers = &authHandlerRegistry{
				handlers: map[string]AuthHandler{
					// "codex" is deliberate: it is the name the retired branch
					// keyed on, and it must no longer decide anything.
					"codex": declaredMethodHandler{methods: test.methods},
				},
				fallback: &acpAuthHandler{},
			}

			if err := w.configureAgent("codex", map[string]string{
				"api_key":     "typed-key",
				"auth_method": test.methods[0].ID,
			}); err != nil {
				t.Fatalf("configureAgent: %v", err)
			}
			override, err := agentcfg.Load(w.storage, "codex")
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			stored := strings.Join(override.Env, "\x00")
			if !strings.Contains(stored, test.wantEnv) {
				t.Fatalf("stored env = %#v, want it to contain %q", override.Env, test.wantEnv)
			}
			if strings.Contains(stored, test.notInEnv) {
				t.Fatalf("stored env = %#v, must not contain %q", override.Env, test.notInEnv)
			}
		})
	}
}
