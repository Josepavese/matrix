package elicitation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestServiceAskRespondRoundTrip(t *testing.T) {
	svc := NewService(time.Second)
	req := middleware.ElicitationRequest{ID: "e1", Mode: middleware.ElicitationModeForm, Message: "Which strategy?"}
	out := make(chan middleware.ElicitationOutcome, 1)
	go func() { out <- svc.Ask(context.Background(), req) }()

	deadline := time.Now().Add(time.Second)
	for !pendingContains(svc, "e1") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !svc.Respond("e1", middleware.AcceptElicitation(map[string]interface{}{"strategy": "balanced"})) {
		t.Fatal("expected Respond to resolve pending elicitation")
	}
	got := <-out
	if got.Action != middleware.ElicitationActionAccept || got.Values["strategy"] != "balanced" {
		t.Fatalf("unexpected outcome: %+v", got)
	}
	if got.ID != "e1" {
		t.Fatalf("outcome must carry the resolved request id, got %q", got.ID)
	}
	if svc.Respond("e1", middleware.DeclineElicitation()) {
		t.Fatal("double Respond must fail")
	}
}

func TestServiceAskTimesOutAsCancel(t *testing.T) {
	svc := NewService(20 * time.Millisecond)
	got := svc.Ask(context.Background(), middleware.ElicitationRequest{ID: "e2", Mode: middleware.ElicitationModeURL, URL: "https://x.example"})
	if got.Action != middleware.ElicitationActionCancel {
		t.Fatalf("expected cancel on timeout, got %q", got.Action)
	}
}

func TestServiceAskContextCancel(t *testing.T) {
	svc := NewService(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan middleware.ElicitationOutcome, 1)
	go func() { out <- svc.Ask(ctx, middleware.ElicitationRequest{Mode: middleware.ElicitationModeForm}) }()
	cancel()
	if got := <-out; got.Action != middleware.ElicitationActionCancel {
		t.Fatalf("expected cancel on ctx end, got %q", got.Action)
	}
}

// waitFor polls until the condition holds, so tests observe asynchronous
// delivery without depending on goroutine scheduling.
func waitFor(t *testing.T, condition func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func pendingContains(svc *Service, id string) bool {
	for _, p := range svc.Pending() {
		if p.Request.ID == id {
			return true
		}
	}
	return false
}

func TestServiceModes(t *testing.T) {
	modes := NewService(0).Modes()
	if len(modes) != 2 || modes[0] != middleware.ElicitationModeForm || modes[1] != middleware.ElicitationModeURL {
		t.Fatalf("unexpected modes: %v", modes)
	}
}

// TestServiceAssignsUniqueIDsForConcurrentSameScope requests two elicitations
// with the same scope key — legitimate when an agent asks twice in one session
// without a tool call. Both must stay independently answerable.
func TestServiceAssignsUniqueIDsForConcurrentSameScope(t *testing.T) {
	svc := NewService(time.Second)
	first := make(chan middleware.ElicitationOutcome, 1)
	second := make(chan middleware.ElicitationOutcome, 1)
	go func() { first <- svc.Ask(context.Background(), middleware.ElicitationRequest{ID: "session:s1"}) }()
	go func() { second <- svc.Ask(context.Background(), middleware.ElicitationRequest{ID: "session:s1"}) }()

	deadline := time.Now().Add(time.Second)
	for len(svc.Pending()) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	pending := svc.Pending()
	if len(pending) != 2 {
		t.Fatalf("expected two independently pending elicitations, got %d", len(pending))
	}
	if pending[0].Request.ID == pending[1].Request.ID {
		t.Fatalf("concurrent requests share an id: %q", pending[0].Request.ID)
	}
	// The two registrations race, so correlate by outcome rather than by
	// goroutine: answering one ID must resolve exactly one waiting call.
	if !svc.Respond(pending[0].Request.ID, middleware.DeclineElicitation()) {
		t.Fatal("first elicitation must be answerable")
	}
	select {
	case got := <-first:
		if got.Action != middleware.ElicitationActionDecline {
			t.Fatalf("unexpected first outcome: %+v", got)
		}
	case got := <-second:
		if got.Action != middleware.ElicitationActionDecline {
			t.Fatalf("unexpected first outcome: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("first answer resolved nothing")
	}
	select {
	case got := <-first:
		t.Fatalf("second elicitation resolved by the wrong answer: %+v", got)
	case got := <-second:
		t.Fatalf("second elicitation resolved by the wrong answer: %+v", got)
	case <-time.After(20 * time.Millisecond):
	}
	if !svc.Respond(pending[1].Request.ID, middleware.CancelElicitation()) {
		t.Fatal("second elicitation must remain answerable")
	}
	select {
	case got := <-first:
		if got.Action != middleware.ElicitationActionCancel {
			t.Fatalf("unexpected second outcome: %+v", got)
		}
	case got := <-second:
		if got.Action != middleware.ElicitationActionCancel {
			t.Fatalf("unexpected second outcome: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("second answer resolved nothing")
	}
}

// TestPendingIsOldestFirst pins the documented ordering.
func TestPendingIsOldestFirst(t *testing.T) {
	svc := NewService(time.Second)
	go svc.Ask(context.Background(), middleware.ElicitationRequest{ID: "first"})
	deadline := time.Now().Add(time.Second)
	for !pendingContains(svc, "first") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	go svc.Ask(context.Background(), middleware.ElicitationRequest{ID: "second"})
	for len(svc.Pending()) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	pending := svc.Pending()
	if len(pending) != 2 || pending[0].Request.ID != "first" || pending[1].Request.ID != "second" {
		t.Fatalf("pending must be oldest first: %+v", pending)
	}
}

// TestServicePublishesLifecycleEvents pins the observer contract channel
// frontends depend on: opened once, resolved exactly once, with the outcome.
func TestServicePublishesLifecycleEvents(t *testing.T) {
	svc := NewService(time.Second)
	var mu sync.Mutex
	var kinds []EventKind
	var resolved []middleware.ElicitationOutcome
	unsubscribe := svc.Subscribe(func(event Event) {
		mu.Lock()
		defer mu.Unlock()
		kinds = append(kinds, event.Kind)
		if event.Kind == EventResolved {
			resolved = append(resolved, event.Outcome)
		}
	})
	defer unsubscribe()

	done := make(chan middleware.ElicitationOutcome, 1)
	go func() { done <- svc.Ask(context.Background(), middleware.ElicitationRequest{ID: "e1"}) }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if pendingContains(svc, "e1") {
			break
		}
		time.Sleep(time.Millisecond)
	}
	svc.Respond("e1", middleware.AcceptElicitation(map[string]interface{}{"x": 1}))
	<-done

	// Events are delivered on the subscriber's own worker, so wait for them
	// rather than assuming they landed before Respond returned.
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(kinds) == 2
	}, "lifecycle events")

	mu.Lock()
	defer mu.Unlock()
	if len(kinds) != 2 || kinds[0] != EventOpened || kinds[1] != EventResolved {
		t.Fatalf("unexpected event sequence: %v", kinds)
	}
	if len(resolved) != 1 || resolved[0].Action != middleware.ElicitationActionAccept {
		t.Fatalf("resolved event must carry the outcome: %+v", resolved)
	}
}

// TestServicePublishesResolvedOnTimeout keeps retraction working for requests
// that expire unansowered.
func TestServicePublishesResolvedOnTimeout(t *testing.T) {
	svc := NewService(20 * time.Millisecond)
	var mu sync.Mutex
	actions := []string{}
	unsubscribe := svc.Subscribe(func(event Event) {
		if event.Kind != EventResolved {
			return
		}
		mu.Lock()
		actions = append(actions, event.Outcome.Action)
		mu.Unlock()
	})
	defer unsubscribe()
	svc.Ask(context.Background(), middleware.ElicitationRequest{ID: "e-timeout"})
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(actions) == 1
	}, "timeout resolution event")

	mu.Lock()
	defer mu.Unlock()
	if len(actions) != 1 || actions[0] != middleware.ElicitationActionCancel {
		t.Fatalf("timeout must publish a cancel resolution: %v", actions)
	}
}

// TestUnsubscribeStopsDelivery keeps a stopped frontend from receiving events.
func TestUnsubscribeStopsDelivery(t *testing.T) {
	svc := NewService(time.Second)
	delivered := 0
	unsubscribe := svc.Subscribe(func(Event) { delivered++ })
	unsubscribe()
	unsubscribe() // idempotent
	go svc.Ask(context.Background(), middleware.ElicitationRequest{ID: "e-nobody"})
	time.Sleep(50 * time.Millisecond)
	if delivered != 0 {
		t.Fatalf("unsubscribed observer received %d events", delivered)
	}
}
