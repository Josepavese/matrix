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

	mu                     sync.Mutex
	promptCalls            int
	firstReturned          bool
	secondSawFirstReturned bool
	firstStarted           chan struct{}
	firstReleased          chan struct{}
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
func (c *blockingACPClient) AuthenticatedProtocolVersion() int   { return supportedACPProtocolVersion }
func (c *blockingACPClient) Initialize(context.Context, acpInitializeRequest) (*acpInitializeResponse, error) {
	return &acpInitializeResponse{ProtocolVersion: supportedACPProtocolVersion}, nil
}
func (c *blockingACPClient) Authenticate(context.Context, string) error {
	return nil
}
func (c *blockingACPClient) Logout(context.Context, acpLogoutRequest) (*acpLogoutResponse, error) {
	return &acpLogoutResponse{}, nil
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
func (c *blockingACPClient) ListSessionsWithRequest(context.Context, acpListSessionsRequest) (*acpListSessionsResponse, error) {
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
func (c *blockingACPClient) ExtRequest(context.Context, string, interface{}, interface{}) error {
	return nil
}
func (c *blockingACPClient) ExtNotification(context.Context, string, interface{}) error {
	return nil
}

func (c *blockingACPClient) Prompt(ctx context.Context, _ acpPromptRequest, _ acpSessionObserver) (*acpPromptResponse, error) {
	call := c.recordPromptCall()
	if call != 1 {
		// A later prompt that arrives while the first has not returned is a
		// serialization violation; record it so the test can name it even when
		// the polling window missed the moment it happened.
		c.mu.Lock()
		c.secondSawFirstReturned = c.firstReturned
		c.mu.Unlock()
		return &acpPromptResponse{}, nil
	}
	close(c.firstStarted)
	select {
	case <-ctx.Done():
		c.markFirstReturned()
		return nil, ctx.Err()
	case <-c.firstReleased:
		c.markFirstReturned()
		return &acpPromptResponse{}, nil
	}
}

// markFirstReturned runs before the first Prompt returns, so the per-session
// guard cannot release the next turn before this flag is visible.
func (c *blockingACPClient) markFirstReturned() {
	c.mu.Lock()
	c.firstReturned = true
	c.mu.Unlock()
}

func (c *blockingACPClient) secondPromptWaitedForFirst() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.secondSawFirstReturned
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

	// Serialization is a negative claim — the second prompt must not be
	// forwarded while the first is in flight — so the test waits for the wait
	// itself: the guard reports the turn blocked on this session. Only after
	// that fact is observed does the silence assertion mean anything, because a
	// second turn that never reached the guard could not have been serialized
	// either way. No clock decides the outcome.
	waitForPromptWaiter(t, client, "remote-session")
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
	if !fake.secondPromptWaitedForFirst() {
		t.Fatal("second prompt was forwarded while the first was still in flight")
	}
	if waiters := client.promptWaiterCount("remote-session"); waiters != 0 {
		t.Fatalf("the guard still reports %d waiting turn(s) after both completed", waiters)
	}
}

// waitForPromptWaiter polls the session guard until the turn blocked on it is
// registered, and fails closed: a wait that never became observable proves
// nothing about serialization, so the caller's silence check must not run.
func waitForPromptWaiter(t *testing.T, client *acpConversationClient, remoteSessionID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if client.promptWaiterCount(remoteSessionID) > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no turn ever registered as waiting on session %q: the second prompt was not serialized", remoteSessionID)
}
