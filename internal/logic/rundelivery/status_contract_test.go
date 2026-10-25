package rundelivery

import (
	"errors"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

var errStorageUnavailable = errors.New("storage unavailable")

// faultStorage is a memstore whose operations can be made to fail one at a
// time, so the store's error propagation is exercised without a real backend.
type faultStorage struct {
	*memstore.Storage
	getErr  error
	setErr  error
	listErr error
}

func (s *faultStorage) Get(key string) ([]byte, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.Storage.Get(key)
}

func (s *faultStorage) Set(key string, value []byte) error {
	if s.setErr != nil {
		return s.setErr
	}
	return s.Storage.Set(key, value)
}

func (s *faultStorage) List(prefix string) ([]string, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.Storage.List(prefix)
}

// TestBackoffForAttemptGrowsAndCaps pins the retry schedule: the first retry
// waits a second, doubles per attempt, and stops doubling at six so a broken
// sink cannot be retried minutes apart.
func TestBackoffForAttemptGrowsAndCaps(t *testing.T) {
	cases := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{"below first attempt", -3, time.Second},
		{"attempt zero", 0, time.Second},
		{"first retry", 1, 2 * time.Second},
		{"second retry", 2, 4 * time.Second},
		{"fifth retry", 5, 32 * time.Second},
		{"cap boundary", 6, 64 * time.Second},
		{"above cap", 7, 64 * time.Second},
		{"far above cap", 42, 64 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := backoffForAttempt(tc.attempt); got != tc.want {
				t.Fatalf("backoffForAttempt(%d) = %v, want %v", tc.attempt, got, tc.want)
			}
		})
	}
}

// TestMarkFailedRecordsAndClearsTheLastError pins both halves of the error
// field contract: a failed attempt records why it failed, and a later attempt
// that reports no error does not keep showing the stale one.
func TestMarkFailedRecordsAndClearsTheLastError(t *testing.T) {
	store := NewStore(memstore.New())
	delivery := newDelivery(t, store, "sink-1")

	if err := store.MarkFailed(delivery.ID, errors.New("sink refused"), 5); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	failed, found, err := store.Load(delivery.ID)
	if err != nil || !found {
		t.Fatalf("load after failure: found=%v err=%v", found, err)
	}
	if failed.LastError != "sink refused" {
		t.Fatalf("expected the failure cause to be recorded, got %q", failed.LastError)
	}

	// The next attempt reports no error of its own: the old cause must not
	// outlive the attempt that produced it.
	if err := store.MarkFailed(delivery.ID, nil, 5); err != nil {
		t.Fatalf("mark failed without error: %v", err)
	}
	recovered, found, err := store.Load(delivery.ID)
	if err != nil || !found {
		t.Fatalf("load after recovery: found=%v err=%v", found, err)
	}
	if recovered.LastError != "" {
		t.Fatalf("a nil delivery error must clear the stale cause, got %q", recovered.LastError)
	}
	if recovered.Attempts != 2 {
		t.Fatalf("expected two attempts after two failures, got %d", recovered.Attempts)
	}
	if recovered.NextAttemptAt.Before(time.Now().UTC().Add(2 * time.Second)) {
		t.Fatalf("second attempt must back off by at least backoffForAttempt(2) (%v), got %v",
			backoffForAttempt(2), recovered.NextAttemptAt)
	}
}

// TestStatusUpdatesPropagateStorageReadErrors keeps a broken backend from being
// reported as a missing delivery: the caller needs to tell "storage failed"
// from "the record is gone", or a transient read error looks like a lost event.
func TestStatusUpdatesPropagateStorageReadErrors(t *testing.T) {
	store := NewStore(&faultStorage{Storage: memstore.New(), getErr: errStorageUnavailable})

	if err := store.MarkSent("delivery-x"); !errors.Is(err, errStorageUnavailable) {
		t.Fatalf("MarkSent must surface the read failure, got %v", err)
	} else if errors.Is(err, ErrDeliveryNotFound) {
		t.Fatal("a read failure must not masquerade as a missing delivery")
	}

	if err := store.MarkFailed("delivery-x", errors.New("boom"), 3); !errors.Is(err, errStorageUnavailable) {
		t.Fatalf("MarkFailed must surface the read failure, got %v", err)
	} else if errors.Is(err, ErrDeliveryNotFound) {
		t.Fatal("a read failure must not masquerade as a missing delivery")
	}
}
