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

func (c *recoveryClient) Initialize(ctx context.Context, req middleware.InitializeRequest) (*middleware.InitializeResponse, error) {
	return &middleware.InitializeResponse{}, nil
}

func (c *recoveryClient) NewSession(ctx context.Context, req middleware.NewSessionRequest) (*middleware.NewSessionResponse, error) {
	c.newSessionCalls++
	return &middleware.NewSessionResponse{SessionId: fmt.Sprintf("session-%d", c.newSessionCalls)}, nil
}

func (c *recoveryClient) Prompt(ctx context.Context, req middleware.PromptRequest, observer middleware.SessionObserver) (*middleware.PromptResponse, error) {
	c.promptCalls++
	if c.promptCalls == 1 {
		return nil, fmt.Errorf("RPC error -32602: Invalid params: Session not found: stale-session")
	}
	if observer != nil {
		observer.OnUpdate(middleware.SessionNotification{
			SessionId: req.SessionId,
			Update: middleware.SessionUpdate{
				SessionUpdate: "agent_message_chunk",
				Content: middleware.Content{
					Type: "text",
					Text: "recovered",
				},
			},
		})
	}
	return &middleware.PromptResponse{StopReason: "end_turn"}, nil
}

func (c *recoveryClient) SetRequestHandler(handler middleware.RequestHandler)         {}
func (c *recoveryClient) SetMode(ctx context.Context, sessionID, modeID string) error { return nil }

func TestRouter_ExecutePrompt_RecoversFromMissingSession(t *testing.T) {
	router := NewRouter(nil)
	client := &recoveryClient{}

	output, sessionID, _, err := router.executePrompt(context.Background(), client, middleware.RouteRequest{
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
		t.Fatalf("expected 1 new session call, got %d", client.newSessionCalls)
	}
	if client.promptCalls != 2 {
		t.Fatalf("expected 2 prompt calls, got %d", client.promptCalls)
	}
}
