package agents

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

type blockingACPClient struct {
	ctx context.Context

	mu            sync.Mutex
	promptCalls   int
	firstStarted  chan struct{}
	firstReleased chan struct{}
}

func newBlockingACPClient(ctx context.Context) *blockingACPClient {
	return &blockingACPClient{
		ctx:           ctx,
		firstStarted:  make(chan struct{}),
		firstReleased: make(chan struct{}),
	}
}

func (c *blockingACPClient) Context() context.Context            { return c.ctx }
func (c *blockingACPClient) Close() error                        { return nil }
func (c *blockingACPClient) SetRequestHandler(acpRequestHandler) {}
func (c *blockingACPClient) Initialize(context.Context, acpInitializeRequest) (*acpInitializeResponse, error) {
	return &acpInitializeResponse{}, nil
}
func (c *blockingACPClient) Authenticate(context.Context, string, map[string]string) error {
	return nil
}
func (c *blockingACPClient) NewSession(context.Context, acpNewSessionRequest) (*acpNewSessionResponse, error) {
	return &acpNewSessionResponse{SessionID: "remote-session"}, nil
}
func (c *blockingACPClient) LoadSession(context.Context, acpLoadSessionRequest, acpSessionObserver) (*acpLoadSessionResponse, error) {
	return &acpLoadSessionResponse{}, nil
}
func (c *blockingACPClient) ResumeSession(context.Context, acpResumeSessionRequest) (*acpResumeSessionResponse, error) {
	return &acpResumeSessionResponse{}, nil
}
func (c *blockingACPClient) ListSessions(context.Context) (*acpListSessionsResponse, error) {
	return &acpListSessionsResponse{}, nil
}
func (c *blockingACPClient) CancelSession(context.Context, string) error { return nil }
func (c *blockingACPClient) CloseSession(context.Context, string) error  { return nil }
func (c *blockingACPClient) DeleteSession(context.Context, string) error { return nil }
func (c *blockingACPClient) ForkSession(context.Context, acpForkSessionRequest) (*acpForkSessionResponse, error) {
	return &acpForkSessionResponse{SessionID: "fork-session"}, nil
}
func (c *blockingACPClient) SetMode(context.Context, string, string) error { return nil }
func (c *blockingACPClient) SetConfigOption(context.Context, acpSetConfigOptionRequest) (*acpSetConfigOptionResponse, error) {
	return &acpSetConfigOptionResponse{}, nil
}

func (c *blockingACPClient) Prompt(ctx context.Context, _ acpPromptRequest, _ acpSessionObserver) (*acpPromptResponse, error) {
	call := c.recordPromptCall()
	if call != 1 {
		return &acpPromptResponse{}, nil
	}
	close(c.firstStarted)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.firstReleased:
		return &acpPromptResponse{}, nil
	}
}

func (c *blockingACPClient) recordPromptCall() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.promptCalls++
	return c.promptCalls
}

func (c *blockingACPClient) PromptCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.promptCalls
}

func TestACPConversationClientRejectsLiveAttachDuringActivePrompt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake := newBlockingACPClient(ctx)
	client := &acpConversationClient{
		client:         fake,
		loadedSessions: map[string]bool{},
	}

	done := make(chan error, 1)
	go func() {
		_, err := client.ExecuteTurn(ctx, middleware.ConversationTurn{
			RemoteSessionID: "remote-session",
			Message:         "long running turn",
		})
		done <- err
	}()

	<-fake.firstStarted
	_, err := client.ExecuteTurn(ctx, middleware.ConversationTurn{
		RemoteSessionID:   "remote-session",
		Message:           "live context",
		LiveContextAttach: true,
	})
	if !errors.Is(err, middleware.ErrConversationTurnActive) {
		t.Fatalf("expected active-turn error, got %v", err)
	}
	if calls := fake.PromptCalls(); calls != 1 {
		t.Fatalf("live attach must not send a second ACP prompt, calls=%d", calls)
	}

	close(fake.firstReleased)
	if err := <-done; err != nil {
		t.Fatalf("first prompt failed: %v", err)
	}
}

func TestACPConversationClientTracksNewRemoteSessionForCleanupOwnership(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake := newBlockingACPClient(ctx)
	client := &acpConversationClient{
		client:         fake,
		loadedSessions: map[string]bool{},
	}

	done := make(chan error, 1)
	go func() {
		_, err := client.ExecuteTurn(ctx, middleware.ConversationTurn{Message: "create remote"})
		done <- err
	}()

	<-fake.firstStarted
	if !clientTracksRemoteSession(client, "remote-session") {
		t.Fatalf("newly created ACP session must be tracked as owned by the client")
	}
	close(fake.firstReleased)
	if err := <-done; err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
}

func TestACPConversationClientSerializesNormalPromptsForSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake := newBlockingACPClient(ctx)
	client := &acpConversationClient{
		client:         fake,
		loadedSessions: map[string]bool{},
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := client.ExecuteTurn(ctx, middleware.ConversationTurn{RemoteSessionID: "remote-session", Message: "first"})
		firstDone <- err
	}()
	<-fake.firstStarted

	secondDone := make(chan error, 1)
	go func() {
		_, err := client.ExecuteTurn(ctx, middleware.ConversationTurn{RemoteSessionID: "remote-session", Message: "second"})
		secondDone <- err
	}()

	time.Sleep(50 * time.Millisecond)
	if calls := fake.PromptCalls(); calls != 1 {
		t.Fatalf("second prompt should wait for first prompt completion, calls=%d", calls)
	}

	close(fake.firstReleased)
	if err := <-firstDone; err != nil {
		t.Fatalf("first prompt failed: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second prompt failed: %v", err)
	}
	if calls := fake.PromptCalls(); calls != 2 {
		t.Fatalf("expected second prompt after first completion, calls=%d", calls)
	}
}
