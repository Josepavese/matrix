package elicitation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestRespondNeverReportsSuccessForADiscardedAnswer is the invariant that keeps
// the HTTP and Telegram surfaces honest: when Respond returns true, the caller
// (and therefore the human) is told the agent received the answer. If Respond
// can win the map race while Ask has already given up on the timeout, the
// registry reports success for an answer that was thrown away.
//
// The window is tiny, so the test hammers it rather than waiting for luck.
func TestRespondNeverReportsSuccessForADiscardedAnswer(t *testing.T) {
	const rounds = 400
	for round := 0; round < rounds; round++ {
		service := NewService(time.Millisecond)
		type result struct {
			outcome middleware.ElicitationOutcome
		}
		asked := make(chan result, 1)
		id := "race-" + time.Now().Format("150405.000000000")
		go func() {
			asked <- result{outcome: service.Ask(context.Background(), middleware.ElicitationRequest{ID: id})}
		}()

		// Wait until it is registered, then answer as the deadline expires.
		deadline := time.Now().Add(50 * time.Millisecond)
		for len(service.Pending()) == 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Microsecond)
		}
		accepted := service.Respond(id, middleware.AcceptElicitation(map[string]interface{}{"v": round}))

		var outcome middleware.ElicitationOutcome
		select {
		case got := <-asked:
			outcome = got.outcome
		case <-time.After(2 * time.Second):
			t.Fatalf("round %d: Ask never returned", round)
		}
		if accepted && outcome.Action != middleware.ElicitationActionAccept {
			t.Fatalf("round %d: Respond reported success but the agent received %q (answer silently discarded)",
				round, outcome.Action)
		}
	}
}

// TestRespondAfterTimeoutIsRejected is the deterministic half of the same
// invariant: once the request expired, answering it must fail loudly.
func TestRespondAfterTimeoutIsRejected(t *testing.T) {
	service := NewService(10 * time.Millisecond)
	outcome := service.Ask(context.Background(), middleware.ElicitationRequest{ID: "expired"})
	if outcome.Action != middleware.ElicitationActionCancel {
		t.Fatalf("expected cancel on timeout, got %q", outcome.Action)
	}
	if service.Respond("expired", middleware.AcceptElicitation(nil)) {
		t.Fatal("answering an expired request must be rejected")
	}
}

// TestConcurrentRespondHasExactlyOneWinner keeps the double-tap path honest:
// two humans (or one impatient human and a timeout) must not both "win".
func TestConcurrentRespondHasExactlyOneWinner(t *testing.T) {
	service := NewService(time.Minute)
	asked := make(chan middleware.ElicitationOutcome, 1)
	go func() {
		asked <- service.Ask(context.Background(), middleware.ElicitationRequest{ID: "tap"})
	}()
	deadline := time.Now().Add(time.Second)
	for len(service.Pending()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	var wg sync.WaitGroup
	wins := make([]bool, 8)
	for i := range wins {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			value := middleware.AcceptElicitation(map[string]interface{}{"who": index})
			wins[index] = service.Respond("tap", value)
		}(i)
	}
	wg.Wait()

	winners := 0
	for _, won := range wins {
		if won {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly one winning answer, got %d", winners)
	}
	select {
	case got := <-asked:
		if got.Action != middleware.ElicitationActionAccept {
			t.Fatalf("agent must receive the winning answer, got %q", got.Action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ask never returned")
	}
}

// TestContextCancellationRacingAnAnswer proves the same guarantee for the
// run-cancel path used by the turn binding.
func TestContextCancellationRacingAnAnswer(t *testing.T) {
	const rounds = 200
	for round := 0; round < rounds; round++ {
		service := NewService(time.Minute)
		ctx, cancel := context.WithCancel(context.Background())
		asked := make(chan middleware.ElicitationOutcome, 1)
		id := "ctx-race"
		go func() { asked <- service.Ask(ctx, middleware.ElicitationRequest{ID: id}) }()
		deadline := time.Now().Add(50 * time.Millisecond)
		for len(service.Pending()) == 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Microsecond)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		var accepted bool
		go func() { defer wg.Done(); accepted = service.Respond(id, middleware.AcceptElicitation(nil)) }()
		go func() { defer wg.Done(); cancel() }()
		wg.Wait()

		var outcome middleware.ElicitationOutcome
		select {
		case outcome = <-asked:
		case <-time.After(2 * time.Second):
			t.Fatalf("round %d: Ask never returned", round)
		}
		if accepted && outcome.Action != middleware.ElicitationActionAccept {
			t.Fatalf("round %d: cancel raced the answer and the accepted answer was lost (agent saw %q)",
				round, outcome.Action)
		}
	}
}

// TestResolvedEntriesDoNotLeak keeps the registry bounded: every terminal path
// must remove its entry.
func TestResolvedEntriesDoNotLeak(t *testing.T) {
	service := NewService(20 * time.Millisecond)
	for i := 0; i < 50; i++ {
		go func() { service.Ask(context.Background(), middleware.ElicitationRequest{ID: "leak"}) }()
	}
	time.Sleep(200 * time.Millisecond)
	if pending := service.Pending(); len(pending) != 0 {
		t.Fatalf("registry leaked %d expired entries", len(pending))
	}
}

// TestManyConcurrentRequestsKeepUniqueIDs guards the collision fix under load.
func TestManyConcurrentRequestsKeepUniqueIDs(t *testing.T) {
	service := NewService(time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.Ask(context.Background(), middleware.ElicitationRequest{ID: "same-scope"})
		}()
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(service.Pending()) == 200 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	seen := map[string]bool{}
	for _, pending := range service.Pending() {
		if seen[pending.Request.ID] {
			t.Fatalf("duplicate pending id %q under load", pending.Request.ID)
		}
		seen[pending.Request.ID] = true
	}
	if len(seen) != 200 {
		t.Fatalf("expected 200 distinct pending ids, got %d", len(seen))
	}
}

// TestSubscriberNeverBlocksTheAgent is the robustness invariant for channel
// frontends: a slow UI (a Telegram API call inside the event handler) must not
// delay the answer reaching the agent or the HTTP response.
func TestSubscriberNeverBlocksTheAgent(t *testing.T) {
	service := NewService(time.Minute)
	release := make(chan struct{})
	service.Subscribe(func(event Event) {
		if event.Kind == EventResolved {
			<-release // simulate a hanging frontend call
		}
	})

	asked := make(chan middleware.ElicitationOutcome, 1)
	go func() {
		asked <- service.Ask(context.Background(), middleware.ElicitationRequest{ID: "slow-ui"})
	}()
	deadline := time.Now().Add(time.Second)
	for len(service.Pending()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	responded := make(chan bool, 1)
	go func() { responded <- service.Respond("slow-ui", middleware.DeclineElicitation()) }()

	select {
	case <-responded:
	case <-time.After(500 * time.Millisecond):
		close(release)
		t.Fatal("a hanging channel frontend blocked Respond: the answer cannot reach the agent")
	}
	close(release)

	select {
	case got := <-asked:
		if got.Action != middleware.ElicitationActionDecline {
			t.Fatalf("agent must receive the decline, got %q", got.Action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ask never returned")
	}
}
