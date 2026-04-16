package agents

import (
	"context"
	"fmt"
	"testing"

	"github.com/jose/matrix-v2/internal/middleware"
)

type recoveryClient struct {
	newSessionCalls int
	promptCalls     int
}

func (c *recoveryClient) Alive() bool  { return true }
func (c *recoveryClient) Close() error { return nil }

func (c *recoveryClient) ExecuteTurn(_ context.Context, turn middleware.ConversationTurn) (middleware.ConversationResult, error) {
	c.promptCalls++
	if c.promptCalls == 1 && turn.RemoteSessionID != "" {
		return middleware.ConversationResult{}, fmt.Errorf("ACP prompt failed: RPC error -32602: Invalid params: Session not found: stale-session")
	}
	c.newSessionCalls++
	return middleware.ConversationResult{
		Output:          "recovered",
		RemoteSessionID: fmt.Sprintf("session-%d", c.newSessionCalls),
	}, nil
}

func TestRouter_ExecutePrompt_RecoversFromMissingSession(t *testing.T) {
	router := NewRouter(nil)
	client := &recoveryClient{}

	output, sessionID, _, _, err := router.executePrompt(context.Background(), client, middleware.RouteRequest{
		LogicalSessionID: "logical-session",
		AgentSessionID:   "stale-session",
		Message:          "hello",
	})
	if err != nil {
		t.Fatalf("executePrompt failed: %v", err)
	}
	if output != "recovered" {
		t.Fatalf("expected recovered output, got %q", output)
	}
	if sessionID != "session-1" {
		t.Fatalf("expected new session id session-1, got %q", sessionID)
	}
	if client.newSessionCalls != 1 {
		t.Fatalf("expected 1 remote session assignment, got %d", client.newSessionCalls)
	}
	if client.promptCalls != 2 {
		t.Fatalf("expected 2 prompt calls, got %d", client.promptCalls)
	}
}
