package agents

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// promptResolver is the optional companion an endpoint resolver may implement.
// It keeps the test focused on the router's decision, not on vault plumbing.
type promptResolver struct {
	endpoint middleware.ProtocolEndpoint
	prompt   middleware.SystemPrompt
}

func (r promptResolver) GetAgentEndpoint(string) (middleware.ProtocolEndpoint, error) {
	return r.endpoint, nil
}

func (r promptResolver) GetAgentSystemPrompt(string) (middleware.SystemPrompt, error) {
	return r.prompt, nil
}

// TestRouterResolvesTheConfiguredSystemPrompt pins where the prompt enters the
// factory: only a resolver that implements the optional companion supplies it,
// and a configured prompt on a protocol that cannot carry it is an error instead
// of a value dropped on the floor.
func TestRouterResolvesTheConfiguredSystemPrompt(t *testing.T) {
	acp := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: "agent"}
	a2a := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindA2A, Transport: "JSONRPC", Address: "https://agent.example/rpc"}
	cases := []struct {
		name     string
		resolver middleware.AgentEndpointResolver
		endpoint middleware.ProtocolEndpoint
		want     middleware.SystemPrompt
		wantErr  string
	}{
		{
			name:     "agent prompt reaches the factory",
			resolver: promptResolver{endpoint: acp, prompt: middleware.SystemPrompt{Mode: middleware.SystemPromptModeAppend, Text: "be concise"}},
			endpoint: acp,
			want:     middleware.SystemPrompt{Mode: middleware.SystemPromptModeAppend, Text: "be concise"},
		},
		{
			name:     "no configured prompt is no prompt",
			resolver: promptResolver{endpoint: acp},
			endpoint: acp,
		},
		{
			name:     "a resolver without the companion leaves it unset",
			resolver: &mockResolver{protocol: "stdio", address: "agent"},
			endpoint: acp,
		},
		{
			name:     "a prompt the A2A protocol cannot carry is an error",
			resolver: promptResolver{endpoint: a2a, prompt: middleware.SystemPrompt{Text: "be concise"}},
			endpoint: a2a,
			wantErr:  "only the ACP protocol carries",
		},
		{
			name:     "an invalid mode is an error",
			resolver: promptResolver{endpoint: acp, prompt: middleware.SystemPrompt{Mode: "replace", Text: "be concise"}},
			endpoint: acp,
			wantErr:  "unsupported system prompt mode",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(tc.resolver)
			defer router.Close()
			got, err := router.systemPromptFor("agent-id", tc.endpoint)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("prompt = %+v, want %+v", got, tc.want)
			}
		})
	}
}
