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

	// notificationAckMaxAgeKey is where an operator sets how long an
	// acknowledgement record may live, in seconds. Absent means no expiry, and
	// that absence is the default on purpose: the window a legitimate replay
	// needs is set by the supervisor's own persisted cursor, which the daemon
	// does not hold, so any number chosen here would be a promise Matrix cannot
	// keep. A key that expired too early turns a replay into a new claim, and
	// the growth is therefore declared rather than bounded by a guess.
	notificationAckMaxAgeKey = "retention.notification_ack_max_age"

	// notificationAckSweepInterval rate-limits the pass that removes expired
	// records. Without a pass the window would only hide records, not bound
	// them; on every acknowledgement the pass would cost more than the
	// acknowledgement it follows.
	notificationAckSweepInterval = time.Minute
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
	// same claim cannot both observe an empty record and both write one. It also
	// serialises the housekeeping sweep below, which reads and deletes the same
	// records.
	s.idempotencyMu.Lock()
	replay, expired, window, err := s.recordNotificationAck(key, req)
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
	body := map[string]interface{}{
		"status":   "acked",
		"run_id":   req.RunID,
		"sequence": req.Sequence,
	}
	// When an operator configured a window, the answer declares it: after that
	// window the same key comes back as a first claim, and a consumer that is not
	// told so reads a replay that turned into a new claim as a retry that never
	// happened. The field is absent when nothing expires, because then nothing
	// can.
	if window > 0 {
		body["idempotency_window_seconds"] = int64(window.Seconds())
	}
	if expired {
		body["idempotency_record_expired"] = true
	}
	writeJSON(w, http.StatusOK, body)
}

func decodeNotificationAck(w http.ResponseWriter, r *http.Request) (notificationAckRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, notificationAckBodyMaxBytes)
	var req notificationAckRequest
	decoder := json.NewDecoder(r.Body)
	// A machine-written body: an unknown field is a typo that would otherwise
	// acknowledge something the caller did not mean.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			// A body that was never read to the end is the size case, not the json
			// one: the same distinction the run submission makes, answered the same
			// way, so one error does not get two answers on two endpoints.
			http.Error(w, fmt.Sprintf("Request Entity Too Large: the acknowledgement body is limited to %d bytes", tooLarge.Limit), http.StatusRequestEntityTooLarge)
			return notificationAckRequest{}, false
		}
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
// a replay of an already recorded one, whether it replaced a record that had
// fallen outside the configured window, and which window is configured.
func (s *Server) recordNotificationAck(key string, req notificationAckRequest) (bool, bool, time.Duration, error) {
	storage := s.storage
	if storage == nil {
		return false, false, 0, fmt.Errorf("run notification acknowledgement storage not available")
	}
	maxAge, err := notificationAckMaxAge(storage)
	if err != nil {
		return false, false, 0, err
	}
	storageKey := notificationAckStorageKey(key)
	previous, found, err := loadNotificationAckRecord(storage, storageKey)
	if err != nil {
		return false, false, 0, err
	}
	// A record past the window is not a record any more: it is what the operator
	// asked Matrix to forget. It is treated as absent rather than as a conflict,
	// because the claim it held is no longer inside the guarantee, and the answer
	// says so instead of letting a replay look like a fresh key.
	expired := found && maxAge > 0 && time.Since(previous.AckedAt) >= maxAge
	if found && !expired {
		if previous.Digest != notificationAckDigest(req) {
			return false, false, 0, errNotificationAckConflict
		}
		return true, false, maxAge, nil
	}
	record := notificationAckRecord{
		RunID:    req.RunID,
		Sequence: req.Sequence,
		Digest:   notificationAckDigest(req),
		AckedAt:  time.Now().UTC(),
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return false, false, 0, err
	}
	if err := storage.Set(storageKey, encoded); err != nil {
		return false, false, 0, err
	}
	s.sweepExpiredNotificationAcks(storage, maxAge)
	return false, expired, maxAge, nil
}

// notificationAckMaxAge reads the configured window. Zero means no expiry, which
// is both the default and the only value Matrix chooses for an operator. A value
// that is present but unusable is an error rather than a silent fallback: an
// operator who wrote a bound down and got an unbounded store instead would have
// been told nothing.
func notificationAckMaxAge(storage middleware.Storage) (time.Duration, error) {
	data, err := storage.Get(notificationAckMaxAgeKey)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", notificationAckMaxAgeKey, err)
	}
	if len(data) == 0 {
		return 0, nil
	}
	var seconds int64
	if err := json.Unmarshal(data, &seconds); err != nil {
		return 0, fmt.Errorf("%s must be a number of seconds: %w", notificationAckMaxAgeKey, err)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("%s must be a positive number of seconds, got %d: remove the key for no expiry", notificationAckMaxAgeKey, seconds)
	}
	return time.Duration(seconds) * time.Second, nil
}

// sweepExpiredNotificationAcks removes records older than the configured window.
// This is what makes the window a bound instead of a promise: a record that is
// only ignored still grows the store, which is the whole complaint against an
// unbounded set of them. The caller holds idempotencyMu, which is what keeps the
// last-sweep mark single-writer, and the pass is best effort: the claim this
// call just wrote is durable, and housekeeping that fails must not turn it into
// a refused one. Records then live until the next pass, which is a late
// collection, not a lost guarantee.
func (s *Server) sweepExpiredNotificationAcks(storage middleware.Storage, maxAge time.Duration) {
	if maxAge <= 0 || time.Since(s.lastAckSweep) < notificationAckSweepInterval {
		return
	}
	s.lastAckSweep = time.Now()
	keys, err := storage.List(notificationAckPrefix)
	if err != nil {
		slog.Warn("notification acknowledgement sweep could not list records", "error", err)
		return
	}
	evicted := 0
	for _, key := range keys {
		record, found, err := loadNotificationAckRecord(storage, key)
		if err != nil || !found || time.Since(record.AckedAt) < maxAge {
			continue
		}
		if err := storage.Delete(key); err == nil {
			evicted++
		}
	}
	if evicted > 0 {
		slog.Info("expired notification acknowledgements removed", "count", evicted)
	}
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
