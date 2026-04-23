package rundelivery

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/google/uuid"
)

const deliveryKeyPrefix = "runtrace.delivery."

type Store struct {
	storage middleware.Storage
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
			return nil, err
		}
		if found && delivery.Status == StatusPending && !delivery.NextAttemptAt.After(now) {
			deliveries = append(deliveries, delivery)
		}
	}
	return deliveries, nil
}

func (s *Store) loadByKey(key string) (Delivery, bool, error) {
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
