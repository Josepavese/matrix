package rundelivery

import (
	"errors"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

// TestSaveRequiresIDAndStampsUpdatedAt pins the two defaults Save owes its
// callers: an anonymous record is rejected, and a record saved without a
// timestamp is stamped instead of being stored with a zero one.
func TestSaveRequiresIDAndStampsUpdatedAt(t *testing.T) {
	store := NewStore(memstore.New())

	if err := store.Save(Delivery{}); err == nil {
		t.Fatal("Save must reject a delivery without an id")
	}

	before := time.Now().UTC()
	if err := store.Save(Delivery{ID: "delivery-no-timestamp"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, found, err := store.Load("delivery-no-timestamp")
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if loaded.UpdatedAt.IsZero() {
		t.Fatal("Save must stamp UpdatedAt when it is unset")
	}
	if loaded.UpdatedAt.Before(before) || loaded.UpdatedAt.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("stamped UpdatedAt %v is outside the save window starting %v", loaded.UpdatedAt, before)
	}
	if !loaded.CreatedAt.IsZero() {
		t.Fatalf("Save must not invent a creation time, got %v", loaded.CreatedAt)
	}
}

// TestSaveAndLoadPropagateStorageErrors proves a backend failure reaches the
// caller wrapped, not swallowed and not relabelled as a missing record.
func TestSaveAndLoadPropagateStorageErrors(t *testing.T) {
	setStore := NewStore(&faultStorage{Storage: memstore.New(), setErr: errStorageUnavailable})
	if err := setStore.Save(Delivery{ID: "delivery-x"}); !errors.Is(err, errStorageUnavailable) {
		t.Fatalf("Save must surface the write failure, got %v", err)
	}

	getStore := NewStore(&faultStorage{Storage: memstore.New(), getErr: errStorageUnavailable})
	if _, _, err := getStore.Load("delivery-x"); !errors.Is(err, errStorageUnavailable) {
		t.Fatalf("Load must surface the read failure, got %v", err)
	}
}

// TestListDuePropagatesStorageErrorsAndHonoursLimit keeps a failing listing
// loud, and keeps a poll bounded when the listing succeeds.
func TestListDuePropagatesStorageErrorsAndHonoursLimit(t *testing.T) {
	broken := NewStore(&faultStorage{Storage: memstore.New(), listErr: errStorageUnavailable})
	if _, err := broken.ListDue(time.Now().UTC(), 1); !errors.Is(err, errStorageUnavailable) {
		t.Fatalf("ListDue must surface the listing failure, got %v", err)
	}

	store := NewStore(memstore.New())
	for i := 0; i < 3; i++ {
		delivery := newDelivery(t, store, "sink-1")
		delivery.ClaimedUntil = time.Time{}
		delivery.NextAttemptAt = time.Now().UTC().Add(-time.Minute)
		if err := store.Save(delivery); err != nil {
			t.Fatal(err)
		}
	}
	due, err := store.ListDue(time.Now().UTC(), 2)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 2 {
		t.Fatalf("ListDue must honour the limit, got %d deliveries", len(due))
	}
}

// TestClaimDueDefaultsTheLease pins the fallback used when a poller passes no
// lease: claims still get the default lease instead of none at all.
func TestClaimDueDefaultsTheLease(t *testing.T) {
	store := NewStore(memstore.New())
	delivery := newDelivery(t, store, "sink-1")
	delivery.ClaimedUntil = time.Time{}
	delivery.NextAttemptAt = time.Now().UTC().Add(-time.Minute)
	if err := store.Save(delivery); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	claimed, err := store.ClaimDue(now, 10, 0)
	if err != nil {
		t.Fatalf("claim due: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != delivery.ID {
		t.Fatalf("expected the due delivery to be claimed, got %+v", claimed)
	}
	if want := now.Add(DefaultClaimLease); !claimed[0].ClaimedUntil.Equal(want) {
		t.Fatalf("a zero lease must fall back to %v, got %v", want, claimed[0].ClaimedUntil)
	}
}

// TestClaimDuePropagatesStorageErrors proves a broken listing or a failed lease
// write stops the claim instead of silently pretending nothing was due.
func TestClaimDuePropagatesStorageErrors(t *testing.T) {
	brokenList := NewStore(&faultStorage{Storage: memstore.New(), listErr: errStorageUnavailable})
	if _, err := brokenList.ClaimDue(time.Now().UTC(), 1, time.Minute); !errors.Is(err, errStorageUnavailable) {
		t.Fatalf("ClaimDue must surface the listing failure, got %v", err)
	}

	// A delivery that is due and readable, but whose lease cannot be written:
	// the claim must fail rather than hand out an unleased record.
	storage := memstore.New()
	store := NewStore(storage)
	delivery := newDelivery(t, store, "sink-1")
	delivery.ClaimedUntil = time.Time{}
	delivery.NextAttemptAt = time.Now().UTC().Add(-time.Minute)
	if err := store.Save(delivery); err != nil {
		t.Fatal(err)
	}
	failingStore := NewStore(&faultStorage{Storage: storage, setErr: errStorageUnavailable})
	claimed, err := failingStore.ClaimDue(time.Now().UTC(), 10, time.Minute)
	if !errors.Is(err, errStorageUnavailable) {
		t.Fatalf("ClaimDue must surface a failed lease write, got %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("a delivery whose lease write failed must not be claimed, got %+v", claimed)
	}
}

// TestClaimableNow pins the claimability contract: pending, due, and free of a
// live lease. Each clause is checked on its own so a regression names itself.
func TestClaimableNow(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name     string
		delivery Delivery
		want     bool
	}{
		{
			name:     "pending and due",
			delivery: Delivery{Status: StatusPending, NextAttemptAt: now.Add(-time.Second)},
			want:     true,
		},
		{
			name:     "already sent",
			delivery: Delivery{Status: StatusSent, NextAttemptAt: now.Add(-time.Second)},
			want:     false,
		},
		{
			name:     "dead",
			delivery: Delivery{Status: StatusDead, NextAttemptAt: now.Add(-time.Second)},
			want:     false,
		},
		{
			name:     "retry not due yet",
			delivery: Delivery{Status: StatusPending, NextAttemptAt: now.Add(time.Second)},
			want:     false,
		},
		{
			name:     "live lease",
			delivery: Delivery{Status: StatusPending, NextAttemptAt: now.Add(-time.Second), ClaimedUntil: now.Add(time.Second)},
			want:     false,
		},
		{
			name:     "expired lease",
			delivery: Delivery{Status: StatusPending, NextAttemptAt: now.Add(-time.Second), ClaimedUntil: now.Add(-time.Second)},
			want:     true,
		},
		{
			name:     "fresh enqueue holds the default lease",
			delivery: Delivery{Status: StatusPending, NextAttemptAt: now, ClaimedUntil: now.Add(DefaultClaimLease)},
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := claimableNow(tc.delivery, now); got != tc.want {
				t.Fatalf("claimableNow = %v, want %v (delivery %+v)", got, tc.want, tc.delivery)
			}
		})
	}
}
