package agents

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

type concurrentCancelACPClient struct {
	*blockingACPClient
	cancel  context.CancelFunc
	phase   string
	started map[string]chan struct{}
	release map[string]chan struct{}
}

func newConcurrentCancelACPClient(phase string) *concurrentCancelACPClient {
	clientCtx, cancel := context.WithCancel(context.Background())
	return &concurrentCancelACPClient{
		blockingACPClient: newBlockingACPClient(clientCtx),
		cancel:            cancel,
		phase:             phase,
		started:           map[string]chan struct{}{"logical-a": make(chan struct{}), "logical-b": make(chan struct{}), "remote-a": make(chan struct{}), "remote-b": make(chan struct{})},
		release:           map[string]chan struct{}{"logical-a": make(chan struct{}), "logical-b": make(chan struct{}), "remote-a": make(chan struct{}), "remote-b": make(chan struct{})},
	}
}

func (c *concurrentCancelACPClient) Close() error {
	c.cancel()
	return nil
}

func (c *concurrentCancelACPClient) NewSession(ctx context.Context, req acpNewSessionRequest) (*acpNewSessionResponse, error) {
	if c.phase != "session/new" {
		return &acpNewSessionResponse{SessionID: "remote-" + strings.TrimPrefix(req.ClientTitle, "logical-")}, nil
	}
	if err := c.wait(ctx, req.ClientTitle); err != nil {
		return nil, err
	}
	return &acpNewSessionResponse{SessionID: "remote-" + strings.TrimPrefix(req.ClientTitle, "logical-")}, nil
}

func (c *concurrentCancelACPClient) Prompt(ctx context.Context, req acpPromptRequest, _ acpSessionObserver) (*acpPromptResponse, error) {
	if c.phase == "session/prompt" {
		if err := c.wait(ctx, req.SessionID); err != nil {
			return nil, err
		}
	}
	return &acpPromptResponse{}, nil
}

func (c *concurrentCancelACPClient) wait(ctx context.Context, key string) error {
	close(c.started[key])
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.Context().Done():
		return fmt.Errorf("client context cancelled")
	case <-c.release[key]:
		return nil
	}
}

func TestRouterConcurrentRunCancelDoesNotPoisonSibling(t *testing.T) {
	for _, phase := range []string{"session/new", "session/prompt"} {
		t.Run(phase, func(t *testing.T) {
			fake := newConcurrentCancelACPClient(phase)
			conversation := &acpConversationClient{
				client:         fake,
				loadedSessions: map[string]bool{"remote-a": true, "remote-b": true},
			}
			router := NewRouter(&mockResolver{protocol: "stdio", address: "codex-acp"})
			router.factory[middleware.ProtocolKindACP] = staticFactory{client: conversation}

			ctxA, cancelA := context.WithCancel(context.Background())
			defer cancelA()
			ctxB, cancelB := context.WithCancel(context.Background())
			defer cancelB()
			doneA := routeConcurrentCancelTestRun(ctxA, router, "a", phase)
			doneB := routeConcurrentCancelTestRun(ctxB, router, "b", phase)
			waitConcurrentCancelStart(t, fake, phase, "a")
			waitConcurrentCancelStart(t, fake, phase, "b")

			cancelB()
			if err := waitConcurrentCancelResult(t, doneB); err == nil || !strings.Contains(strings.ToLower(err.Error()), "cancel") {
				t.Fatalf("cancelled run B returned %v", err)
			}
			if err := fake.Context().Err(); err != nil {
				t.Fatalf("shared ACP client closed while sibling A remained active: %v", err)
			}
			reaped, err := router.ReapAgentSessionClient(context.Background(), "codex", "remote-a", "/tmp/shared-workspace")
			if err != nil || reaped {
				t.Fatalf("draining client must not expose process-reap proof: reaped=%v err=%v", reaped, err)
			}
			select {
			case err := <-doneA:
				t.Fatalf("sibling A terminated during B cancellation: %v", err)
			case <-time.After(50 * time.Millisecond):
			}

			close(fake.release[phaseKey(phase, "a")])
			if err := waitConcurrentCancelResult(t, doneA); err != nil {
				t.Fatalf("sibling A failed after B cancellation: %v", err)
			}
			select {
			case <-fake.Context().Done():
			case <-time.After(time.Second):
				t.Fatal("draining ACP client was not closed after final sibling completed")
			}
			reaped, err = router.ReapAgentSessionClient(context.Background(), "codex", "remote-a", "/tmp/shared-workspace")
			if err != nil || !reaped {
				t.Fatalf("final client close must expose matching process-reap proof: reaped=%v err=%v", reaped, err)
			}
		})
	}
}

func routeConcurrentCancelTestRun(ctx context.Context, router *Router, suffix, phase string) <-chan error {
	done := make(chan error, 1)
	go func() {
		req := middleware.RouteRequest{
			AgentID:          "codex",
			LogicalSessionID: "logical-" + suffix,
			WorkspacePath:    "/tmp/shared-workspace",
			Message:          "run " + suffix,
		}
		if phase == "session/prompt" {
			req.AgentSessionID = "remote-" + suffix
		}
		_, _, _, _, err := router.Route(ctx, req)
		done <- err
	}()
	return done
}

func waitConcurrentCancelStart(t *testing.T, client *concurrentCancelACPClient, phase, suffix string) {
	t.Helper()
	select {
	case <-client.started[phaseKey(phase, suffix)]:
	case <-time.After(time.Second):
		t.Fatalf("%s run %s did not start", phase, suffix)
	}
}

func waitConcurrentCancelResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("run did not terminate")
		return nil
	}
}

func phaseKey(phase, suffix string) string {
	if phase == "session/new" {
		return "logical-" + suffix
	}
	return "remote-" + suffix
}
