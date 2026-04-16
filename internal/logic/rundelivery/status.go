package rundelivery

import "time"

func (s *Store) MarkSent(deliveryID string) error {
	delivery, found, err := s.Load(deliveryID)
	if err != nil || !found {
		return err
	}
	delivery.Status = StatusSent
	delivery.UpdatedAt = time.Now().UTC()
	return s.Save(delivery)
}

func (s *Store) MarkFailed(deliveryID string, deliveryErr error, maxAttempts int) error {
	delivery, found, err := s.Load(deliveryID)
	if err != nil || !found {
		return err
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
