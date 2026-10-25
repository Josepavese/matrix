package elicitation

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestSubscribeNilObserverIsNoOp keeps a caller that has nothing to observe
// from registering a worker, and proves the registry still serves requests
// with no observer attached.
func TestSubscribeNilObserverIsNoOp(t *testing.T) {
	svc := NewService(time.Second)
	unsubscribe := svc.Subscribe(nil)
	if unsubscribe == nil {
		t.Fatal("Subscribe(nil) must return a usable unsubscribe function")
	}
	unsubscribe()
	unsubscribe() // idempotent

	if registered := subscriberCount(svc); registered != 0 {
		t.Fatalf("a nil observer must not be registered, got %d subscriber(s)", registered)
	}

	out := make(chan middleware.ElicitationOutcome, 1)
	go func() { out <- svc.Ask(context.Background(), middleware.ElicitationRequest{ID: "e-no-observer"}) }()
	waitFor(t, func() bool { return pendingContains(svc, "e-no-observer") }, "request registration")
	if !svc.Respond("e-no-observer", middleware.AcceptElicitation(nil)) {
		t.Fatal("Respond must resolve the request with no observer registered")
	}
	select {
	case got := <-out:
		if got.Action != middleware.ElicitationActionAccept {
			t.Fatalf("unexpected outcome with no observer: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ask never returned after Respond")
	}
}

// TestServicePublishDropsWhenSubscriberQueueFull pins the back-pressure
// contract: a channel frontend that is stuck inside its handler cannot make
// the registry block, and the excess events are dropped for that subscriber
// only. The subscriber's worker is parked on the first event, so exactly the
// buffered events plus the in-flight one may still be delivered.
func TestServicePublishDropsWhenSubscriberQueueFull(t *testing.T) {
	svc := NewService(time.Minute)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()

	var delivered atomic.Int64
	var once sync.Once
	unsubscribe := svc.Subscribe(func(Event) {
		once.Do(func() { close(entered) })
		<-release
		delivered.Add(1)
	})
	defer unsubscribe()

	// The first event parks the subscriber's worker inside its handler.
	svc.publish(Event{Kind: EventOpened, Request: middleware.ElicitationRequest{ID: "e-first"}})
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber never received the first event")
	}

	// Fill the subscriber's queue and then overflow it. publish runs in a
	// goroutine so a blocking implementation fails the test instead of
	// wedging the whole suite.
	const overflow = 5
	published := make(chan struct{})
	go func() {
		for i := 0; i < subscriberBuffer+overflow; i++ {
			svc.publish(Event{Kind: EventOpened, Request: middleware.ElicitationRequest{ID: fmt.Sprintf("e-%d", i)}})
		}
		close(published)
	}()
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on a full subscriber queue")
	}
	unblock()

	want := int64(1 + subscriberBuffer)
	waitFor(t, func() bool { return delivered.Load() == want }, "buffered events to drain")
	// A dropped event must never surface later, when the queue has room again.
	time.Sleep(20 * time.Millisecond)
	if got := delivered.Load(); got != want {
		t.Fatalf("expected exactly %d delivered events (in-flight + buffer), got %d", want, got)
	}
}

// TestServiceAskInitialisesPendingRegistry proves Ask still works on a Service
// that was built as a literal instead of through NewService: the registry is
// created on demand, so a missing constructor call degrades to a working
// service rather than a nil-map panic.
func TestServiceAskInitialisesPendingRegistry(t *testing.T) {
	svc := &Service{timeout: time.Minute}
	out := make(chan middleware.ElicitationOutcome, 1)
	go func() { out <- svc.Ask(context.Background(), middleware.ElicitationRequest{ID: "e-lazy"}) }()

	waitFor(t, func() bool { return pendingContains(svc, "e-lazy") }, "request registration in a lazily created registry")
	if !svc.Respond("e-lazy", middleware.DeclineElicitation()) {
		t.Fatal("Respond must resolve a request registered in a lazily created registry")
	}
	select {
	case got := <-out:
		if got.Action != middleware.ElicitationActionDecline {
			t.Fatalf("unexpected outcome from a lazily created registry: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ask never returned after Respond")
	}
}
