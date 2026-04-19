package runapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jose/matrix-v2/internal/logic/runaction"
	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/middleware"
)

func (s *Server) HandleRunResource(w http.ResponseWriter, r *http.Request) {
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	runID, resource, ok := parseRunResource(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch resource {
	case "trace":
		s.handleRunTrace(w, r, runID)
	case "events":
		if wantsEventStream(r) {
			s.streamRunEvents(w, r, runID)
			return
		}
		s.handleRunEvents(w, r, runID)
	case "actions":
		s.handleRunActions(w, r, runID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleRunTrace(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	trace, found, err := s.runStore.Trace(runID)
	if err != nil {
		slog.Error("matrix run trace read failed", "error", err, "run_id", runID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, trace)
}

func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"))
	after := strings.TrimSpace(r.URL.Query().Get("after"))
	if _, found, err := s.runStore.LoadRun(runID); err != nil {
		slog.Error("matrix run read failed", "error", err, "run_id", runID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	} else if !found {
		http.NotFound(w, r)
		return
	}
	events, err := s.runStore.LoadEventsAfter(runID, after, limit)
	if err != nil {
		slog.Error("matrix run events read failed", "error", err, "run_id", runID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"run_id": runID, "events": events, "next_cursor": nextCursor(events)})
}

func wantsEventStream(r *http.Request) bool {
	return r.URL.Query().Get("stream") == "sse" || strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

func (s *Server) streamRunEvents(w http.ResponseWriter, r *http.Request, runID string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	cursor := strings.TrimSpace(r.URL.Query().Get("after"))
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		done, next := s.writeSSEBatch(w, runID, cursor)
		cursor = next
		if done {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) writeSSEBatch(w http.ResponseWriter, runID, cursor string) (bool, string) {
	events, err := s.runStore.LoadEventsAfter(runID, cursor, 100)
	if err != nil {
		_, _ = fmt.Fprintf(w, "event: error\ndata: %q\n\n", err.Error())
		return true, cursor
	}
	for _, event := range events {
		payload, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", event.ID, event.Kind, payload)
		cursor = event.ID
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	run, found, _ := s.runStore.LoadRun(runID)
	return found && isTerminalRun(run.Status), cursor
}

func isTerminalRun(status string) bool {
	switch status {
	case runtrace.StatusCompleted, runtrace.StatusFailed, runtrace.StatusCancelled:
		return true
	default:
		return false
	}
}

func nextCursor(events []runtrace.Event) string {
	if len(events) == 0 {
		return ""
	}
	return events[len(events)-1].ID
}

func (s *Server) handleRunActions(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req runaction.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return
	}
	attacher, _ := s.router.(middleware.RunContextAttacher)
	code, resp := runaction.New(s.runStore, attacher, s.cancelActiveRun).Handle(r.Context(), runID, req)
	if code >= http.StatusInternalServerError {
		slog.Error("matrix run action failed", "run_id", runID, "action", req.Action, "message", resp.Message)
	}
	writeJSON(w, code, resp)
}

func (s *Server) trackRunCancel(runID string, cancel context.CancelFunc) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.runCancels[runID] = cancel
}

func (s *Server) untrackRunCancel(runID string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	delete(s.runCancels, runID)
}

func (s *Server) cancelActiveRun(runID string) {
	s.runMu.Lock()
	cancel := s.runCancels[runID]
	delete(s.runCancels, runID)
	s.runMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
