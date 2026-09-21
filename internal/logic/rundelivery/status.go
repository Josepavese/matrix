package rundelivery

import (
	"fmt"
	"time"
)

// ErrDeliveryNotFound is returned when a status update targets a delivery that
// is no longer stored. Reporting success here would hide a lost delivery.
var ErrDeliveryNotFound = fmt.Errorf("delivery not found")

func (s *Store) MarkSent(deliveryID string) error {
	delivery, found, err := s.Load(deliveryID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrDeliveryNotFound, deliveryID)
	}
	delivery.Status = StatusSent
	delivery.ClaimedUntil = time.Time{}
	delivery.UpdatedAt = time.Now().UTC()
	return s.Save(delivery)
}

func (s *Store) MarkFailed(deliveryID string, deliveryErr error, maxAttempts int) error {
	delivery, found, err := s.Load(deliveryID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrDeliveryNotFound, deliveryID)
	}
	delivery.Attempts++
	delivery.LastError = ""
	if deliveryErr != nil {
		delivery.LastError = deliveryErr.Error()
	}
	delivery.Status = StatusPending
	if delivery.Attempts >= maxAttempts {
		delivery.Status = StatusDead
	}
	// Release the lease so the backoff, not the lease, decides the retry time.
	delivery.ClaimedUntil = time.Time{}
	delivery.NextAttemptAt = time.Now().UTC().Add(backoffForAttempt(delivery.Attempts))
	delivery.UpdatedAt = time.Now().UTC()
	return s.Save(delivery)
}

func backoffForAttempt(attempt int) time.Duration {
	if attempt < 1 {
		return time.Second
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<attempt) * time.Second
}
