package rundelivery

import (
	"errors"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

// ----------------------------------------------------------------------------
// Adversarial delivery store suite
// ----------------------------------------------------------------------------

func newDelivery(t *testing.T, store *Store, sinkID string) Delivery {
	t.Helper()
	delivery, err := store.Enqueue(runtrace.Sink{ID: sinkID, URL: "https://sink.example/ingest"}, runtrace.Event{
		ID: "event-1", RunID: "run-1", Kind: "agent.message.delta",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return delivery
}

// TestClaimDueTakesEachDeliveryOnce is the regression guard for duplicate sink
// posts: a poll interval shorter than a send must not hand the same delivery to
// two workers.
func TestClaimDueTakesEachDeliveryOnce(t *testing.T) {
	store := NewStore(memstore.New())
	delivery := newDelivery(t, store, "sink-1")

	now := time.Now().UTC()
	// The immediate attempt is already in flight: a poll before the lease
	// expires must not take the same record again.
	if claimed, err := store.ClaimDue(now, 10, time.Minute); err != nil || len(claimed) != 0 {
		t.Fatalf("a fresh delivery must not be claimable twice: %v %v", claimed, err)
	}
	// After the lease expires the record is recoverable (the worker died).
	later := now.Add(2 * time.Minute)
	claimed, err := store.ClaimDue(later, 10, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("expected the expired lease to be reclaimable: %v %v", claimed, err)
	}
	if claimed[0].ID != delivery.ID {
		t.Fatalf("claimed the wrong delivery: %s", claimed[0].ID)
	}
	// And it is leased again, not handed out repeatedly.
	if again, err := store.ClaimDue(later, 10, time.Minute); err != nil || len(again) != 0 {
		t.Fatalf("a claim must be exclusive: %v %v", again, err)
	}
}

// TestClaimDueNeverHandsOutTheSameDeliveryConcurrently exercises the claim under
// parallel polling.
func TestClaimDueNeverHandsOutTheSameDeliveryConcurrently(t *testing.T) {
	store := NewStore(memstore.New())
	for i := 0; i < 20; i++ {
		newDelivery(t, store, "sink-1")
	}
	// Age every record past any lease.
	for _, delivery := range listAll(t, store) {
		delivery.ClaimedUntil = time.Time{}
		delivery.NextAttemptAt = time.Now().UTC().Add(-time.Minute)
		if err := store.Save(delivery); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().UTC()
	seen := make(chan string, 200)
	done := make(chan struct{})
	for worker := 0; worker < 8; worker++ {
		go func() {
			defer func() { done <- struct{}{} }()
			claimed, err := store.ClaimDue(now, 5, time.Minute)
			if err != nil {
				return
			}
			for _, delivery := range claimed {
				seen <- delivery.ID
			}
		}()
	}
	for worker := 0; worker < 8; worker++ {
		<-done
	}
	close(seen)

	counts := map[string]int{}
	for id := range seen {
		counts[id]++
	}
	for id, count := range counts {
		if count > 1 {
			t.Fatalf("delivery %s was claimed %d times", id, count)
		}
	}
}

// TestMarkSentReleasesTheLease keeps a completed delivery from looking busy.
func TestMarkSentReleasesTheLease(t *testing.T) {
	store := NewStore(memstore.New())
	delivery := newDelivery(t, store, "sink-1")
	if err := store.MarkSent(delivery.ID); err != nil {
		t.Fatalf("mark sent: %v", err)
	}
	stored, found, err := store.Load(delivery.ID)
	if err != nil || !found {
		t.Fatalf("load: %v %v", found, err)
	}
	if stored.Status != StatusSent {
		t.Fatalf("expected sent status, got %s", stored.Status)
	}
	if !stored.ClaimedUntil.IsZero() {
		t.Fatal("a sent delivery must not keep a lease")
	}
}

// TestMarkFailedReleasesTheLeaseAndSchedulesRetry pins the retry contract: the
// backoff, not the lease, decides when the next attempt happens.
func TestMarkFailedReleasesTheLeaseAndSchedulesRetry(t *testing.T) {
	store := NewStore(memstore.New())
	delivery := newDelivery(t, store, "sink-1")
	if err := store.MarkFailed(delivery.ID, errors.New("sink refused"), 5); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	stored, _, err := store.Load(delivery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusPending {
		t.Fatalf("expected a pending retry, got %s", stored.Status)
	}
	if stored.Attempts != 1 {
		t.Fatalf("expected one attempt, got %d", stored.Attempts)
	}
	if !stored.ClaimedUntil.IsZero() {
		t.Fatal("a failed delivery must release its lease")
	}
	if !stored.NextAttemptAt.After(time.Now().UTC()) {
		t.Fatal("a retry must be scheduled in the future")
	}
}

// TestMarkFailedMarksDeadAtMaxAttempts keeps a broken sink from being retried
// forever.
func TestMarkFailedMarksDeadAtMaxAttempts(t *testing.T) {
	store := NewStore(memstore.New())
	delivery := newDelivery(t, store, "sink-1")
	if err := store.MarkFailed(delivery.ID, errors.New("nope"), 1); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	stored, _, err := store.Load(delivery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusDead {
		t.Fatalf("expected dead status at max attempts, got %s", stored.Status)
	}
}

// TestStatusUpdatesOnAMissingDeliveryReportFailure keeps a lost record from
// looking like a successful update.
func TestStatusUpdatesOnAMissingDeliveryReportFailure(t *testing.T) {
	store := NewStore(memstore.New())
	if err := store.MarkSent("does-not-exist"); !errors.Is(err, ErrDeliveryNotFound) {
		t.Fatalf("MarkSent must report a missing delivery, got %v", err)
	}
	if err := store.MarkFailed("does-not-exist", nil, 3); !errors.Is(err, ErrDeliveryNotFound) {
		t.Fatalf("MarkFailed must report a missing delivery, got %v", err)
	}
}

// TestCorruptRecordDoesNotBlockOtherDeliveries keeps one bad value in storage
// from stopping every sink forever.
func TestCorruptRecordDoesNotBlockOtherDeliveries(t *testing.T) {
	storage := memstore.New()
	store := NewStore(storage)
	good := newDelivery(t, store, "sink-1")
	if err := storage.Set(DeliveryKey("delivery-corrupt"), []byte("{not json")); err != nil {
		t.Fatal(err)
	}

	// Age the healthy record so it is due.
	stored, _, err := store.Load(good.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.ClaimedUntil = time.Time{}
	stored.NextAttemptAt = time.Now().UTC().Add(-time.Minute)
	if err := store.Save(stored); err != nil {
		t.Fatal(err)
	}

	due, err := store.ListDue(time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("a corrupt record must not fail the whole listing: %v", err)
	}
	if len(due) != 1 || due[0].ID != good.ID {
		t.Fatalf("expected the healthy delivery to be listed, got %+v", due)
	}
	claimed, err := store.ClaimDue(time.Now().UTC(), 10, time.Minute)
	if err != nil {
		t.Fatalf("claim must tolerate an unreadable record: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != good.ID {
		t.Fatalf("expected the healthy delivery to be claimed, got %+v", claimed)
	}
}

// TestNilStorageIsReportedNotPanicked keeps a misconfigured store from taking
// down the daemon.
func TestNilStorageIsReportedNotPanicked(t *testing.T) {
	store := NewStore(nil)
	if _, err := store.ListDue(time.Now().UTC(), 1); err == nil {
		t.Fatal("ListDue must report missing storage")
	}
	if _, err := store.ClaimDue(time.Now().UTC(), 1, time.Minute); err == nil {
		t.Fatal("ClaimDue must report missing storage")
	}
	if _, _, err := store.Load("x"); err == nil {
		t.Fatal("Load must report missing storage")
	}
	if err := store.Save(Delivery{ID: "x"}); err == nil {
		t.Fatal("Save must report missing storage")
	}
}

// TestClaimDueRespectsTheLimit keeps a poll from doing unbounded work.
func TestClaimDueRespectsTheLimit(t *testing.T) {
	store := NewStore(memstore.New())
	for i := 0; i < 5; i++ {
		delivery := newDelivery(t, store, "sink-1")
		delivery.ClaimedUntil = time.Time{}
		delivery.NextAttemptAt = time.Now().UTC().Add(-time.Minute)
		if err := store.Save(delivery); err != nil {
			t.Fatal(err)
		}
	}
	claimed, err := store.ClaimDue(time.Now().UTC(), 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 2 {
		t.Fatalf("expected the limit to be honoured, got %d", len(claimed))
	}
}

func listAll(t *testing.T, store *Store) []Delivery {
	t.Helper()
	deliveries, err := store.ListDue(time.Now().UTC().Add(time.Hour), 1000)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return deliveries
}
