package runtrace

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrIdempotencyConflict = errors.New("idempotency key already used with a different request")

const idempotencyKeyPrefix = "runtrace.idempotency."

type idempotencyRecord struct {
	RunID     string    `json:"run_id"`
	Digest    string    `json:"digest"`
	CreatedAt time.Time `json:"created_at"`
}

// ReserveRunID persists the deduplication decision before a run can be
// dispatched. A reservation without a run is intentionally left ambiguous:
// retrying must never create a second potentially mutating execution.
func (s *Store) ReserveRunID(scope, key, digest, proposedID string) (string, bool, error) {
	if s == nil || s.storage == nil {
		return "", false, fmt.Errorf("run trace storage not available")
	}
	s.idempotencyMu.Lock()
	defer s.idempotencyMu.Unlock()
	keyHash := sha256.Sum256([]byte(scope + "\x00" + key))
	storageKey := fmt.Sprintf("%s%x", idempotencyKeyPrefix, keyHash)
	data, err := s.storage.Get(storageKey)
	if err != nil {
		return "", false, err
	}
	if len(data) != 0 {
		var previous idempotencyRecord
		if err := json.Unmarshal(data, &previous); err != nil {
			return "", false, fmt.Errorf("invalid idempotency record: %w", err)
		}
		if previous.Digest != digest {
			return previous.RunID, true, ErrIdempotencyConflict
		}
		return previous.RunID, true, nil
	}
	record := idempotencyRecord{RunID: proposedID, Digest: digest, CreatedAt: time.Now().UTC()}
	data, err = json.Marshal(record)
	if err != nil {
		return "", false, err
	}
	if err := s.storage.Set(storageKey, data); err != nil {
		return "", false, err
	}
	return proposedID, false, nil
}
