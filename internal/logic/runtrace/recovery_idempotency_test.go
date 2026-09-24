package runtrace

import (
	"errors"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

func TestIdempotencyReservationSurvivesStoreRecreation(t *testing.T) {
	storage := memstore.New()
	first := NewStore(storage)
	id, replay, err := first.ReserveRunID("channel", "key", "digest-a", "run-original")
	if err != nil || replay || id != "run-original" {
		t.Fatalf("first reservation: %q %v %v", id, replay, err)
	}
	second := NewStore(storage)
	id, replay, err = second.ReserveRunID("channel", "key", "digest-a", "run-duplicate")
	if err != nil || !replay || id != "run-original" {
		t.Fatalf("replay: %q %v %v", id, replay, err)
	}
	_, _, err = second.ReserveRunID("channel", "key", "digest-b", "run-conflict")
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestRecoverInterruptedRunsPreservesUncertainOutcome(t *testing.T) {
	storage := memstore.New()
	store := NewStore(storage)
	running, _, err := store.Start(Run{ChannelID: "ch", AgentID: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	completed, _, err := store.Start(Run{ChannelID: "ch", AgentID: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Complete(completed.ID, "done", "end_turn"); err != nil {
		t.Fatal(err)
	}
	restarted := NewStore(storage)
	count, err := restarted.RecoverInterruptedRuns()
	if err != nil || count != 1 {
		t.Fatalf("recovery count=%d err=%v", count, err)
	}
	recovered, found, err := restarted.LoadRun(running.ID)
	if err != nil || !found || recovered.Status != StatusUnknown || recovered.StopReason != "daemon_interrupted" {
		t.Fatalf("recovered=%+v found=%v err=%v", recovered, found, err)
	}
	if again, err := restarted.RecoverInterruptedRuns(); err != nil || again != 0 {
		t.Fatalf("non-idempotent recovery: %d %v", again, err)
	}
	events, err := restarted.LoadEvents(running.ID, 0)
	if err != nil || events[len(events)-1].Kind != "run.outcome_unknown" {
		t.Fatalf("recovery event=%+v err=%v", events, err)
	}
}
