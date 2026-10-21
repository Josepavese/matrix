package zedacp

import (
	"context"
	"errors"
	"testing"
	"time"
)

type heldObserver struct {
	*recordingObserver
	entered chan struct{}
	release chan struct{}
}

func (o *heldObserver) OnUpdate(n SessionNotification) {
	if n.Update.Content.Text == "hold" {
		close(o.entered)
		<-o.release
	}
	o.recordingObserver.OnUpdate(n)
}

func heldPrompt(t *testing.T) (*Client, *fakeTransport, *heldObserver, <-chan error) {
	t.Helper()
	transport := newFakeTransport()
	client := NewClient(context.Background(), transport)
	t.Cleanup(func() { _ = client.Close() })
	observer := &heldObserver{recordingObserver: &recordingObserver{}, entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := client.Prompt(context.Background(), PromptRequest{SessionID: "s"}, observer)
		done <- err
	}()
	waitForSent(t, transport, func(_ string) bool { return true }, "prompt")
	client.handleNotification(sessionUpdateResponse(t, "s", "hold"))
	select {
	case <-observer.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("observer never entered")
	}
	t.Cleanup(func() { close(observer.release) })
	return client, transport, observer, done
}

func TestPromptWaitsForSlowDeliveryBeforeUnregister(t *testing.T) {
	client, transport, observer, done := heldPrompt(t)
	client.handleNotification(sessionUpdateResponse(t, "s", "final"))
	transport.push(t, `{"jsonrpc":"2.0","id":1,"result":{"stopReason":"end_turn"}}`)
	// Longer than both old 250ms drains. The held callback makes the backlog
	// deterministic: neither it nor the final can finish before release.
	select {
	case err := <-done:
		t.Fatalf("prompt returned before delivering final: %v", err)
	case <-time.After(3 * responseObserverDrainIdle):
	}
	observer.release <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not finish after delivery")
	}
	if got := observer.joined(); got != "holdfinal" {
		t.Fatalf("lost output: %q", got)
	}
}

func TestPromptRejectsNotificationOverflow(t *testing.T) {
	client, _, _, done := heldPrompt(t)
	for i := 0; i < notificationQueueBuffer+1; i++ {
		client.enqueueSessionUpdate(SessionNotification{SessionID: "s", Update: SessionUpdate{SessionUpdate: "tool_call_update"}}, 1)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotificationOverflow) {
			t.Fatalf("overflow error lost: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("overflow did not fail the waiting prompt")
	}
}

func TestPromptDrainCancellationDoesNotLeakIntoNextTurn(t *testing.T) {
	transport := newFakeTransport()
	client := NewClient(context.Background(), transport)
	defer client.Close()
	observer := &heldObserver{recordingObserver: &recordingObserver{}, entered: make(chan struct{}), release: make(chan struct{})}
	defer close(observer.release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := client.Prompt(ctx, PromptRequest{SessionID: "s"}, observer); done <- err }()
	waitForSent(t, transport, func(string) bool { return true }, "prompt")
	client.handleNotification(sessionUpdateResponse(t, "s", "hold"))
	select {
	case <-observer.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("callback not entered")
	}
	client.handleNotification(sessionUpdateResponse(t, "s", "old-final"))
	transport.push(t, `{"jsonrpc":"2.0","id":1,"result":{"stopReason":"end_turn"}}`)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not release prompt")
	}
	next := &recordingObserver{}
	stop := client.WatchSession("s", next)
	defer stop()
	client.handleNotification(sessionUpdateResponse(t, "s", "next-final"))
	observer.release <- struct{}{}
	waitForObserverText(t, "next-final", next)
}

func TestFailedConnectionRejectsNewPromptWithoutSending(t *testing.T) {
	transport := newFakeTransport()
	client := NewClient(context.Background(), transport)
	defer client.Close()
	client.fail(ErrNotificationOverflow)
	_, err := client.Prompt(context.Background(), PromptRequest{SessionID: "s"}, nil)
	if !errors.Is(err, ErrNotificationOverflow) {
		t.Fatalf("failure cause lost: %v", err)
	}
	if got := transport.sentPayloads(); len(got) != 0 {
		t.Fatalf("failed connection submitted work: %v", got)
	}
}
