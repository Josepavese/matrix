package runapi

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/google/uuid"
)

// acceptNewRun serializes the durable reservation and run record. Dispatch
// starts only after the lock is released, so a racing retry sees the same ID.
func (s *Server) acceptNewRun(w http.ResponseWriter, r *http.Request, req runRequest, agentID string) (runtrace.Run, bool) {
	s.idempotencyMu.Lock()
	defer s.idempotencyMu.Unlock()
	runID, replay, ok := s.reserveRequestedRun(w, r, req, agentID)
	if !ok {
		return runtrace.Run{}, false
	}
	if replay {
		s.writeReplayedRun(w, runID)
		return runtrace.Run{}, false
	}
	run, err := s.startRun(req, agentID, runID)
	if err != nil {
		slog.Error("matrix run trace start failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return runtrace.Run{}, false
	}
	return run, true
}

func (s *Server) reserveRequestedRun(w http.ResponseWriter, r *http.Request, req runRequest, agentID string) (string, bool, bool) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		return "", false, true
	}
	if len(key) > 128 {
		http.Error(w, "Bad Request: Idempotency-Key exceeds 128 bytes", http.StatusBadRequest)
		return "", false, false
	}
	payload, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "Bad Request: cannot encode run request", http.StatusBadRequest)
		return "", false, false
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(append([]byte(agentID+"\x00"), payload...)))
	runID, replay, err := s.runStore.ReserveRunID(req.ChannelID, key, digest, "run-"+uuid.NewString())
	if errors.Is(err, runtrace.ErrIdempotencyConflict) {
		writeJSON(w, http.StatusConflict, map[string]string{"code": "idempotency_payload_conflict", "run_id": runID, "message": "Idempotency-Key belongs to a different request"})
		return "", false, false
	}
	if err != nil {
		http.Error(w, "Internal Server Error: idempotency reservation failed", http.StatusInternalServerError)
		return "", false, false
	}
	return runID, replay, true
}

func (s *Server) writeReplayedRun(w http.ResponseWriter, runID string) {
	previous, found, err := s.runStore.LoadRun(runID)
	if err != nil || !found {
		writeJSON(w, http.StatusConflict, map[string]string{"code": "acceptance_uncertain", "run_id": runID, "message": "Run reservation exists without a durable run record; inspect provider state before retrying"})
		return
	}
	w.Header().Set("Idempotency-Replayed", "true")
	writeJSON(w, http.StatusAccepted, runResponseBuilder.NewSuccess(runID, previous.Status, previous.Output))
}
