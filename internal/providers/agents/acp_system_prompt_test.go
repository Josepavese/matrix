package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestACPSessionPromptFailsClosedBeforeCreatingASession pins the boundary: a
// configured prompt on an agent that does not advertise
// sessionCapabilities.systemPrompt is an error, not a silent fallback to the
// agent's own instructions, and no session is created before it is reported.
func TestACPSessionPromptFailsClosedBeforeCreatingASession(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{
		client:         fake,
		deps:           middleware.ConversationFactoryDeps{SystemPrompt: middleware.SystemPrompt{Text: "answer briefly"}},
		loadedSessions: map[string]bool{},
	}
	_, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "work"})
	if err == nil || !strings.Contains(err.Error(), "does not advertise sessionCapabilities.systemPrompt") {
		t.Fatalf("expected fail-closed prompt error, got %v", err)
	}
	if fake.newReq != nil || fake.promptReq != nil {
		t.Fatalf("unsupported prompt reached the provider: new=%+v prompt=%+v", fake.newReq, fake.promptReq)
	}
}

// TestACPSessionPromptSendsConfiguredTextOnCreation checks the wire form of the
// stable systemPrompt field on session/new, including the default append mode.
func TestACPSessionPromptSendsConfiguredTextOnCreation(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{
		client:              fake,
		deps:                middleware.ConversationFactoryDeps{SystemPrompt: middleware.SystemPrompt{Text: "answer briefly"}},
		sessionCapabilities: middleware.ConversationSessionCapabilities{SystemPrompt: true},
		loadedSessions:      map[string]bool{},
	}
	if _, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "work"}); err != nil {
		t.Fatalf("execute turn: %v", err)
	}
	if fake.newReq == nil || fake.newReq.SystemPrompt == nil {
		t.Fatalf("session/new did not carry the configured prompt: %+v", fake.newReq)
	}
	prompt := fake.newReq.SystemPrompt
	if prompt.Mode != acpSystemPromptMode(middleware.SystemPromptModeAppend) {
		t.Fatalf("default mode = %q, want append", prompt.Mode)
	}
	if len(prompt.Content) != 1 || prompt.Content[0].Type != "text" || prompt.Content[0].Text != "answer briefly" {
		t.Fatalf("prompt content = %+v", prompt.Content)
	}
}

// TestACPSessionPromptRequiresAdvertisedOverrideMode keeps the replacement mode
// from being sent to an agent that only advertised append.
func TestACPSessionPromptRequiresAdvertisedOverrideMode(t *testing.T) {
	fake := &pagedListACPClient{ctx: context.Background()}
	client := &acpConversationClient{
		client:              fake,
		deps:                middleware.ConversationFactoryDeps{SystemPrompt: middleware.SystemPrompt{Mode: middleware.SystemPromptModeOverride, Text: "be exact"}},
		sessionCapabilities: middleware.ConversationSessionCapabilities{SystemPrompt: true},
		loadedSessions:      map[string]bool{},
	}
	_, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "work"})
	if err == nil || !strings.Contains(err.Error(), "append system prompts only") {
		t.Fatalf("expected override-mode error, got %v", err)
	}
	if fake.newReq != nil {
		t.Fatalf("unadvertised override mode reached session/new: %+v", fake.newReq)
	}
}

// TestACPSessionPromptSendsConfiguredTextOnRestore covers the two lifecycle
// requests Matrix uses to restore a session for a turn, with the prompt on both
// wires and the advertised replacement mode.
func TestACPSessionPromptSendsConfiguredTextOnRestore(t *testing.T) {
	cases := []struct {
		name       string
		capability middleware.ConversationSessionCapabilities
		inspect    func(*pagedListACPClient) *acpSystemPrompt
	}{
		{
			name:       "resume",
			capability: middleware.ConversationSessionCapabilities{Resume: true, SystemPrompt: true, SystemPromptOverride: true},
			inspect: func(fake *pagedListACPClient) *acpSystemPrompt {
				if fake.resumeReq == nil {
					return nil
				}
				return fake.resumeReq.SystemPrompt
			},
		},
		{
			name:       "load",
			capability: middleware.ConversationSessionCapabilities{Load: true, SystemPrompt: true, SystemPromptOverride: true},
			inspect: func(fake *pagedListACPClient) *acpSystemPrompt {
				if fake.loadReq == nil {
					return nil
				}
				return fake.loadReq.SystemPrompt
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &pagedListACPClient{ctx: context.Background()}
			client := &acpConversationClient{
				client:              fake,
				deps:                middleware.ConversationFactoryDeps{SystemPrompt: middleware.SystemPrompt{Mode: middleware.SystemPromptModeOverride, Text: "be exact"}},
				sessionCapabilities: tc.capability,
				loadedSessions:      map[string]bool{},
			}
			if _, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{Message: "work", RemoteSessionID: "remote-1"}); err != nil {
				t.Fatalf("execute turn: %v", err)
			}
			prompt := tc.inspect(fake)
			if prompt == nil {
				t.Fatalf("restore request did not carry the configured prompt: resume=%+v load=%+v", fake.resumeReq, fake.loadReq)
			}
			if prompt.Mode != acpSystemPromptMode(middleware.SystemPromptModeOverride) || len(prompt.Content) != 1 || prompt.Content[0].Text != "be exact" {
				t.Fatalf("restore prompt = %+v", prompt)
			}
		})
	}
}

// TestACPSessionPromptCapabilityIsProjected pins that the neutral capability
// report names the stable system-prompt surface the agent advertised.
func TestACPSessionPromptCapabilityIsProjected(t *testing.T) {
	resp := &acpInitializeResponse{Capabilities: map[string]interface{}{
		"sessionCapabilities": map[string]interface{}{
			"systemPrompt": map[string]interface{}{"mode": map[string]interface{}{"override": map[string]interface{}{}}},
		},
	}}
	caps := acpSessionCapabilities(resp)
	if !caps.SystemPrompt || !caps.SystemPromptOverride {
		t.Fatalf("system prompt capability not projected: %+v", caps)
	}
	if detail := caps.Details["system_prompt"]; !detail.Supported || detail.Stability != "stable" {
		t.Fatalf("system_prompt detail = %+v", detail)
	}
}
