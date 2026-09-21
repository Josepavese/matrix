package rundelivery

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/google/uuid"
)

const deliveryKeyPrefix = "runtrace.delivery."

// DefaultClaimLease bounds how long a worker may hold a delivery before another
// attempt may take it. It must exceed the sink POST timeout so a slow but
// healthy send is never duplicated, and stay short enough that a crashed worker
// does not stall the event for long.
const DefaultClaimLease = 30 * time.Second

type Store struct {
	storage middleware.Storage
	// claimMu serialises claims inside the process, so two workers cannot take
	// the same record between the read and the write.
	claimMu sync.Mutex
}

func NewStore(storage middleware.Storage) *Store {
	return &Store{storage: storage}
}

func DeliveryKey(deliveryID string) string {
	return deliveryKeyPrefix + deliveryID
}

func (s *Store) Enqueue(sink runtrace.Sink, event runtrace.Event) (Delivery, error) {
	now := time.Now().UTC()
	delivery := Delivery{
		ID:            "delivery-" + uuid.NewString(),
		SinkID:        sink.ID,
		RunID:         event.RunID,
		EventID:       event.ID,
		EventKind:     event.Kind,
		Status:        StatusPending,
		NextAttemptAt: now,
		ClaimedUntil:  now.Add(DefaultClaimLease),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return delivery, s.Save(delivery)
}

func (s *Store) Save(delivery Delivery) error {
	if s == nil || s.storage == nil {
		return fmt.Errorf("delivery storage not available")
	}
	if strings.TrimSpace(delivery.ID) == "" {
		return fmt.Errorf("delivery id is required")
	}
	if delivery.UpdatedAt.IsZero() {
		delivery.UpdatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(delivery)
	if err != nil {
		return fmt.Errorf("failed to encode delivery %s: %w", delivery.ID, err)
	}
	if err := s.storage.Set(DeliveryKey(delivery.ID), payload); err != nil {
		return fmt.Errorf("failed to store delivery %s: %w", delivery.ID, err)
	}
	return nil
}

func (s *Store) Load(deliveryID string) (Delivery, bool, error) {
	return s.loadByKey(DeliveryKey(deliveryID))
}

func (s *Store) ListDue(now time.Time, limit int) ([]Delivery, error) {
	if s == nil || s.storage == nil {
		return nil, fmt.Errorf("delivery storage not available")
	}
	keys, err := s.storage.List(deliveryKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list deliveries: %w", err)
	}
	deliveries, err := s.loadDue(keys, now)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(deliveries) > limit {
		deliveries = deliveries[:limit]
	}
	return deliveries, nil
}

func (s *Store) loadDue(keys []string, now time.Time) ([]Delivery, error) {
	deliveries := make([]Delivery, 0, len(keys))
	for _, key := range keys {
		delivery, found, err := s.loadByKey(key)
		if err != nil {
			// A single unreadable record must not stop every other delivery:
			// quarantine it in the logs and keep going.
			slog.Warn("skipping unreadable run event delivery", "event", "delivery_unreadable", "key", key, "error", err)
			continue
		}
		if found && delivery.Status == StatusPending && !delivery.NextAttemptAt.After(now) {
			deliveries = append(deliveries, delivery)
		}
	}
	return deliveries, nil
}

// ClaimDue atomically takes up to limit due deliveries by stamping a lease on
// them, so a long-running send cannot be picked up twice by a later poll.
func (s *Store) ClaimDue(now time.Time, limit int, lease time.Duration) ([]Delivery, error) {
	if s == nil || s.storage == nil {
		return nil, fmt.Errorf("delivery storage not available")
	}
	if lease <= 0 {
		lease = DefaultClaimLease
	}
	s.claimMu.Lock()
	defer s.claimMu.Unlock()

	keys, err := s.storage.List(deliveryKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list deliveries: %w", err)
	}
	claimed := make([]Delivery, 0, len(keys))
	for _, key := range keys {
		if limit > 0 && len(claimed) >= limit {
			break
		}
		delivery, ok, err := s.claimOne(key, now, lease)
		if err != nil {
			return claimed, err
		}
		if ok {
			claimed = append(claimed, delivery)
		}
	}
	return claimed, nil
}

// claimOne stamps a lease on a single delivery when it is due and unclaimed.
func (s *Store) claimOne(key string, now time.Time, lease time.Duration) (Delivery, bool, error) {
	delivery, found, err := s.loadByKey(key)
	if err != nil {
		slog.Warn("skipping unreadable run event delivery", "event", "delivery_unreadable", "key", key, "error", err)
		return Delivery{}, false, nil
	}
	if !found || !claimableNow(delivery, now) {
		return Delivery{}, false, nil
	}
	delivery.ClaimedUntil = now.Add(lease)
	delivery.UpdatedAt = now
	if err := s.Save(delivery); err != nil {
		return Delivery{}, false, err
	}
	return delivery, true, nil
}

// claimableNow reports whether a delivery may be taken by a worker: it must be
// pending, due, and not held under a live lease.
func claimableNow(delivery Delivery, now time.Time) bool {
	if delivery.Status != StatusPending {
		return false
	}
	if delivery.NextAttemptAt.After(now) {
		return false
	}
	return delivery.ClaimedUntil.IsZero() || !delivery.ClaimedUntil.After(now)
}

func (s *Store) loadByKey(key string) (Delivery, bool, error) {
	if s == nil || s.storage == nil {
		return Delivery{}, false, fmt.Errorf("delivery storage not available")
	}
	data, err := s.storage.Get(key)
	if err != nil {
		return Delivery{}, false, fmt.Errorf("failed to read delivery %s: %w", key, err)
	}
	if len(data) == 0 {
		return Delivery{}, false, nil
	}
	var delivery Delivery
	if err := json.Unmarshal(data, &delivery); err != nil {
		return Delivery{}, false, fmt.Errorf("failed to decode delivery %s: %w", key, err)
	}
	return delivery, true, nil
}
