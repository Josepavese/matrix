package runtrace

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

// ----------------------------------------------------------------------------
// Adversarial runtrace suite
// ----------------------------------------------------------------------------

func startTestRun(t *testing.T, store *Store) Run {
	t.Helper()
	run, _, err := store.Start(Run{
		AgentID:       "codex",
		Protocol:      "acp",
		WorkspaceID:   "repo-main",
		ChannelID:     "http.test",
		ExecutionMode: ExecutionModeSync,
		InputRef:      "matrix://runs/pending/input",
		TracePolicy:   TracePolicy{ContentMode: ContentModeRefs, RedactionProfile: "default"},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	return run
}

// TestEventSequencesStayUniquePastTheIndexCap is the regression guard for the
// sequence counter: the event index is capped, so deriving the sequence from its
// length repeated numbers once a run crossed the cap — and duplicate sequences
// are indistinguishable for every consumer that pages by sequence.
func TestEventSequencesStayUniquePastTheIndexCap(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)

	const total = maxRunEventRefs + 250
	sequences := make([]int, 0, total)
	for i := 0; i < total; i++ {
		event, err := store.AppendEvent(Event{RunID: run.ID, Kind: "agent.message.delta", ProtocolMeta: map[string]interface{}{"i": i}})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		sequences = append(sequences, event.Sequence)
	}

	// Start already wrote events, so compare against the first appended number
	// rather than against 1.
	baseline := sequences[0] - 1
	seen := map[int]bool{}
	for index, sequence := range sequences {
		if sequence != baseline+index+1 {
			t.Fatalf("event %d got sequence %d, expected %d", index, sequence, baseline+index+1)
		}
		if seen[sequence] {
			t.Fatalf("sequence %d was reused", sequence)
		}
		seen[sequence] = true
	}
}

// TestEventSequenceSurvivesAStaleCounter covers an upgraded vault: a run with
// events but no counter must continue numbering instead of restarting at one.
func TestEventSequenceSurvivesAStaleCounter(t *testing.T) {
	storage := memstore.New()
	store := NewStore(storage)
	run := startTestRun(t, store)

	var last int
	for i := 0; i < 3; i++ {
		event, err := store.AppendEvent(Event{RunID: run.ID, Kind: "agent.message.delta"})
		if err != nil {
			t.Fatalf("append: %v", err)
		}
		last = event.Sequence
	}
	// Simulate a vault written before the counter existed.
	if err := storage.Delete(RunEventSequenceKey(run.ID)); err != nil {
		t.Fatalf("delete counter: %v", err)
	}
	event, err := store.AppendEvent(Event{RunID: run.ID, Kind: "run.completed"})
	if err != nil {
		t.Fatalf("append after counter loss: %v", err)
	}
	if event.Sequence != last+1 {
		t.Fatalf("expected the sequence to continue at %d, got %d", last+1, event.Sequence)
	}
}

// TestExplicitSequenceRaisesTheCounter keeps a caller-supplied number from
// colliding with the next generated one.
func TestExplicitSequenceRaisesTheCounter(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)

	if _, err := store.AppendEvent(Event{RunID: run.ID, Kind: "custom", Sequence: 42}); err != nil {
		t.Fatalf("append explicit: %v", err)
	}
	event, err := store.AppendEvent(Event{RunID: run.ID, Kind: "agent.message.delta"})
	if err != nil {
		t.Fatalf("append generated: %v", err)
	}
	if event.Sequence != 43 {
		t.Fatalf("expected 43 after an explicit 42, got %d", event.Sequence)
	}
}

// TestDispatcherPanicDoesNotStopDelivery keeps one bad sink from silencing the
// trace: the worker must survive a panicking dispatcher and keep ordering.
func TestDispatcherPanicDoesNotStopDelivery(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)

	var mu sync.Mutex
	delivered := []int{}
	want := []int{}
	panicked := false
	store.WithEventDispatcher(func(event Event) {
		mu.Lock()
		delivered = append(delivered, event.Sequence)
		shouldPanic := !panicked
		panicked = true
		mu.Unlock()
		if shouldPanic {
			panic("sink exploded")
		}
	})

	for i := 0; i < 5; i++ {
		event, err := store.AppendEvent(Event{RunID: run.ID, Kind: "agent.message.delta"})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		want = append(want, event.Sequence)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := len(delivered)
		mu.Unlock()
		if count >= 5 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(delivered) != 5 {
		t.Fatalf("dispatcher stopped after a panic: delivered %v, want %v", delivered, want)
	}
	for index, sequence := range delivered {
		if sequence != want[index] {
			t.Fatalf("events must reach the sink in order, got %v, want %v", delivered, want)
		}
	}
}

// TestDispatcherDropIsSurvivable proves a slow sink cannot block the run that
// produces events.
func TestDispatcherDropIsSurvivable(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)

	release := make(chan struct{})
	store.WithEventDispatcher(func(Event) { <-release })

	// Just past the dispatch buffer while the sink is stuck: enough to take the
	// drop path without making the test slow under the race detector.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < runEventDispatchBuffer+64; i++ {
			if _, err := store.AppendEvent(Event{RunID: run.ID, Kind: "agent.message.delta"}); err != nil {
				t.Errorf("append %d: %v", i, err)
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		close(release)
		t.Fatal("AppendEvent blocked on a stalled dispatcher")
	}
	close(release)
}

// TestEventIndexCapIsDocumentedBehaviour pins what the cap does today so a
// future change to paging is a deliberate decision, not an accident.
func TestEventIndexCapIsDocumentedBehaviour(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)
	for i := 0; i < maxRunEventRefs+10; i++ {
		if _, err := store.AppendEvent(Event{RunID: run.ID, Kind: "agent.message.delta"}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	ids, err := store.loadEventIndex(run.ID)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if len(ids) != maxRunEventRefs {
		t.Fatalf("expected the index to be capped at %d, got %d", maxRunEventRefs, len(ids))
	}
}

// TestConcurrentAppendsGetDistinctSequences exercises the counter under
// concurrency: two appends must never share a number.
func TestConcurrentAppendsGetDistinctSequences(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)

	const writers = 8
	const perWriter = 25
	var mu sync.Mutex
	seen := map[int]bool{}
	var wg sync.WaitGroup
	for writer := 0; writer < writers; writer++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				event, err := store.AppendEvent(Event{RunID: run.ID, Kind: "agent.message.delta"})
				if err != nil {
					t.Errorf("append: %v", err)
					return
				}
				mu.Lock()
				if seen[event.Sequence] {
					t.Errorf("sequence %d reused", event.Sequence)
				}
				seen[event.Sequence] = true
				mu.Unlock()
			}
		}(writer)
	}
	wg.Wait()
	if len(seen) != writers*perWriter {
		t.Fatalf("expected %d distinct sequences, got %d", writers*perWriter, len(seen))
	}
}

// TestTerminalTransitionsAreAtomic races the three terminal transitions against
// each other: cancel-via-HTTP against complete-via-run-loop is the real
// interleaving, and exactly one of them may win.
func TestTerminalTransitionsAreAtomic(t *testing.T) {
	for round := 0; round < 25; round++ {
		store := NewStore(memstore.New())
		run := startTestRun(t, store)

		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make(chan Run, 3)
		wg.Add(3)
		go func() {
			defer wg.Done()
			<-start
			result, err := store.Complete(run.ID, "done", "")
			if err == nil {
				results <- result
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			result, err := store.Cancel(run.ID, "user cancelled")
			if err == nil {
				results <- result
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			result, err := store.Fail(run.ID, context.DeadlineExceeded)
			if err == nil {
				results <- result
			}
		}()
		close(start)
		wg.Wait()
		close(results)

		stored, found, err := store.LoadRun(run.ID)
		if err != nil || !found {
			t.Fatalf("round %d: load: %v %v", round, found, err)
		}
		if !isTerminalStatus(stored.Status) {
			t.Fatalf("round %d: run ended in %q, expected a terminal status", round, stored.Status)
		}
		// Every caller must observe the same winning status.
		for result := range results {
			if result.Status != stored.Status {
				t.Fatalf("round %d: caller saw %q but storage says %q", round, result.Status, stored.Status)
			}
		}
		// Exactly one terminal event may exist.
		events, err := store.LoadEvents(run.ID, 0)
		if err != nil {
			t.Fatalf("round %d: load events: %v", round, err)
		}
		terminalEvents := 0
		for _, event := range events {
			switch event.Kind {
			case "run.completed", "run.failed", "run.cancelled":
				terminalEvents++
			}
		}
		if terminalEvents != 1 {
			t.Fatalf("round %d: expected exactly one terminal event, got %d", round, terminalEvents)
		}
	}
}

// TestTerminalTransitionsAreIdempotent keeps a retried transition from
// appending a second terminal event.
func TestTerminalTransitionsAreIdempotent(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)

	if _, err := store.Complete(run.ID, "done", ""); err != nil {
		t.Fatalf("complete: %v", err)
	}
	second, err := store.Complete(run.ID, "done again", "")
	if err != nil {
		t.Fatalf("second complete must be a no-op, got %v", err)
	}
	if second.Status != StatusCompleted || second.Output != "done" {
		t.Fatalf("a retried transition must return the stored record, got %+v", second)
	}
	// A failed or cancelled transition after completion must not rewrite it.
	if _, err := store.Fail(run.ID, context.DeadlineExceeded); err != nil {
		t.Fatalf("fail after complete: %v", err)
	}
	if _, err := store.Cancel(run.ID, "late"); err != nil {
		t.Fatalf("cancel after complete: %v", err)
	}
	stored, _, err := store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusCompleted {
		t.Fatalf("a completed run lost its status: %s", stored.Status)
	}
	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	terminalEvents := 0
	for _, event := range events {
		switch event.Kind {
		case "run.completed", "run.failed", "run.cancelled":
			terminalEvents++
		}
	}
	if terminalEvents != 1 {
		t.Fatalf("expected one terminal event after retries, got %d", terminalEvents)
	}
}

// TestConcurrentCancelsProduceOneTransition keeps a cancel storm from writing
// several cancelled events.
func TestConcurrentCancelsProduceOneTransition(t *testing.T) {
	store := NewStore(memstore.New())
	run := startTestRun(t, store)

	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.Cancel(run.ID, "stop"); err != nil {
				t.Errorf("cancel: %v", err)
			}
		}()
	}
	wg.Wait()

	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	cancelled := 0
	for _, event := range events {
		if event.Kind == "run.cancelled" {
			cancelled++
		}
	}
	if cancelled != 1 {
		t.Fatalf("expected one run.cancelled event, got %d", cancelled)
	}
}

// TestEvictedEventsDoNotAccumulateInStorage keeps a long run from growing the
// vault forever: once an event leaves the retained window its payload must be
// gone, while the newest events stay readable.
func TestEvictedEventsDoNotAccumulateInStorage(t *testing.T) {
	storage := memstore.New()
	store := NewStore(storage)
	run := startTestRun(t, store)

	firstID := ""
	for i := 0; i < maxRunEventRefs+50; i++ {
		event, err := store.AppendEvent(Event{RunID: run.ID, Kind: "agent.message.delta"})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if firstID == "" {
			firstID = event.ID
		}
	}

	// The first event is outside the window: its payload must be gone.
	if _, found, err := store.LoadEvent(run.ID, firstID); err != nil {
		t.Fatalf("load evicted event: %v", err)
	} else if found {
		t.Fatal("an evicted event payload is still stored")
	}

	// The retained window must still be readable and capped.
	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	if len(events) != maxRunEventRefs {
		t.Fatalf("expected the retained window of %d events, got %d", maxRunEventRefs, len(events))
	}
	// The newest event must be in the window.
	last, err := store.AppendEvent(Event{RunID: run.ID, Kind: "run.completed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.LoadEvent(run.ID, last.ID); err != nil || !found {
		t.Fatalf("the newest event must remain readable: found=%v err=%v", found, err)
	}
}
