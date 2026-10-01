package runapi

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// The delivery contract is at-least-once: the daemon can prove a wakeup was
// written, never that a supervisor delivered it. An acknowledgement is the
// supervisor's own claim about one delivered wakeup, and the only property the
// daemon can enforce is that a claim is recorded exactly once. The idempotency
// key is what makes that true across retries and restarts: the record is
// durable, the replay of the same key returns the recorded outcome instead of
// writing a second one, and the same key with a different claim is a conflict
// rather than a silent overwrite.
const (
	// notificationAckPrefix namespaces acknowledgement records in the shared
	// store, next to run traces and their own idempotency records.
	notificationAckPrefix = "notification_ack."
	// notificationAckScope keeps this idempotency space apart from run
	// submission, so one key can be used for both without collision.
	notificationAckScope = "run-notification-ack"
	// notificationAckKeyMaxBytes mirrors the run submission limit. An
	// unbounded key is a client-provided value written into durable storage.
	notificationAckKeyMaxBytes = 128
	// notificationAckBodyMaxBytes bounds a machine-written request body.
	notificationAckBodyMaxBytes = 4 << 10
)

var errNotificationAckConflict = errors.New("idempotency key already used with a different acknowledgement")

type notificationAckRequest struct {
	RunID    string `json:"run_id"`
	Sequence uint64 `json:"sequence"`
}

type notificationAckRecord struct {
	RunID    string    `json:"run_id"`
	Sequence uint64    `json:"sequence"`
	Digest   string    `json:"digest"`
	AckedAt  time.Time `json:"acked_at"`
}

// HandleLocalNotificationAck records that a supervisor delivered one wakeup.
// The response body is identical for a first acknowledgement and for a replay:
// the outcome is the same, and only the Idempotency-Replayed header says
// whether the daemon had already recorded it.
func (s *Server) HandleLocalNotificationAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireAPIKey(w, r, s.apiKey) {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"code":    "idempotency_key_required",
			"message": "an acknowledgement needs an Idempotency-Key header: it is what makes the claim exactly-once",
		})
		return
	}
	if len(key) > notificationAckKeyMaxBytes {
		http.Error(w, "Bad Request: Idempotency-Key exceeds 128 bytes", http.StatusBadRequest)
		return
	}
	req, ok := decodeNotificationAck(w, r)
	if !ok {
		return
	}
	// The lock makes the read-check-write atomic, so two racing retries of the
	// same claim cannot both observe an empty record and both write one.
	s.idempotencyMu.Lock()
	replay, err := s.recordNotificationAck(key, req)
	s.idempotencyMu.Unlock()
	if errors.Is(err, errNotificationAckConflict) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"code":    "idempotency_payload_conflict",
			"message": "Idempotency-Key belongs to a different acknowledgement",
		})
		return
	}
	if err != nil {
		slog.Error("run notification acknowledgement failed", "error", err)
		http.Error(w, "Internal Server Error: acknowledgement failed", http.StatusInternalServerError)
		return
	}
	if replay {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "acked",
		"run_id":   req.RunID,
		"sequence": req.Sequence,
	})
}

func decodeNotificationAck(w http.ResponseWriter, r *http.Request) (notificationAckRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, notificationAckBodyMaxBytes)
	var req notificationAckRequest
	decoder := json.NewDecoder(r.Body)
	// A machine-written body: an unknown field is a typo that would otherwise
	// acknowledge something the caller did not mean.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return notificationAckRequest{}, false
	}
	if strings.TrimSpace(req.RunID) == "" {
		http.Error(w, "Bad Request: run_id is required", http.StatusBadRequest)
		return notificationAckRequest{}, false
	}
	if req.Sequence == 0 {
		http.Error(w, "Bad Request: sequence must be a positive notification sequence", http.StatusBadRequest)
		return notificationAckRequest{}, false
	}
	return req, true
}

// recordNotificationAck stores the claim once and reports whether this call was
// a replay of an already recorded one.
func (s *Server) recordNotificationAck(key string, req notificationAckRequest) (bool, error) {
	storage := s.storage
	if storage == nil {
		return false, fmt.Errorf("run notification acknowledgement storage not available")
	}
	storageKey := notificationAckStorageKey(key)
	previous, found, err := loadNotificationAckRecord(storage, storageKey)
	if err != nil {
		return false, err
	}
	if found {
		if previous.Digest != notificationAckDigest(req) {
			return false, errNotificationAckConflict
		}
		return true, nil
	}
	record := notificationAckRecord{
		RunID:    req.RunID,
		Sequence: req.Sequence,
		Digest:   notificationAckDigest(req),
		AckedAt:  time.Now().UTC(),
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	if err := storage.Set(storageKey, encoded); err != nil {
		return false, err
	}
	return false, nil
}

// loadNotificationAck exposes the durable record behind one key. It exists so
// the replay proof can show the record is answered from storage and never
// rewritten, and so an operator can inspect what Matrix kept.
func (s *Server) loadNotificationAck(key string) (notificationAckRecord, bool, error) {
	if s.storage == nil {
		return notificationAckRecord{}, false, fmt.Errorf("run notification acknowledgement storage not available")
	}
	return loadNotificationAckRecord(s.storage, notificationAckStorageKey(key))
}

func loadNotificationAckRecord(storage middleware.Storage, storageKey string) (notificationAckRecord, bool, error) {
	data, err := storage.Get(storageKey)
	if err != nil {
		return notificationAckRecord{}, false, err
	}
	if len(data) == 0 {
		return notificationAckRecord{}, false, nil
	}
	var record notificationAckRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return notificationAckRecord{}, false, fmt.Errorf("invalid notification acknowledgement record: %w", err)
	}
	return record, true, nil
}

// notificationAckStorageKey hashes the key, so an opaque client-chosen value
// never becomes a storage key an operator has to read.
func notificationAckStorageKey(key string) string {
	sum := sha256.Sum256([]byte(notificationAckScope + "\x00" + key))
	return fmt.Sprintf("%s%x", notificationAckPrefix, sum)
}

// notificationAckDigest binds a key to the one claim it may carry.
func notificationAckDigest(req notificationAckRequest) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", req.RunID, req.Sequence))))
}
