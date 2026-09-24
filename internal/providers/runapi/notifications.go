package runapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

// HandleLocalNotifications serves a content-minimal, durable cursor on the
// Unix socket only. A waiting supervisor consumes no agent turns while idle.
func (s *Server) HandleLocalNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireAPIKey(w, r, s.apiKey) {
		return
	}
	after, runIDs, ok := parseNotificationQuery(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("stream") == "sse" {
		s.streamLocalNotifications(w, r, after, runIDs)
		return
	}
	items, cursor, err := s.runStore.LoadNotificationsAfter(after, 100, runIDs)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"notifications": items, "next_cursor": cursor})
}

func parseNotificationQuery(w http.ResponseWriter, r *http.Request) (uint64, map[string]struct{}, bool) {
	rawCursor := strings.TrimSpace(r.URL.Query().Get("after"))
	if rawCursor == "" {
		rawCursor = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	after := uint64(0)
	if rawCursor != "" {
		var err error
		after, err = strconv.ParseUint(rawCursor, 10, 64)
		if err != nil {
			http.Error(w, "Bad Request: after must be a notification sequence", http.StatusBadRequest)
			return 0, nil, false
		}
	}
	runIDs := map[string]struct{}{}
	for _, id := range r.URL.Query()["run_id"] {
		id = strings.TrimSpace(id)
		if id != "" {
			runIDs[id] = struct{}{}
		}
	}
	return after, runIDs, true
}

func (s *Server) streamLocalNotifications(w http.ResponseWriter, r *http.Request, after uint64, runIDs map[string]struct{}) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = fmt.Fprint(w, ": connected\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		items, cursor, err := s.runStore.LoadNotificationsAfter(after, 100, runIDs)
		if err != nil {
			_, _ = fmt.Fprintf(w, "event: error\ndata: %q\n\n", err.Error())
			return
		}
		writeNotificationSSEItems(w, items)
		if len(items) != 0 {
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		after = cursor
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func writeNotificationSSEItems(w http.ResponseWriter, items []runtrace.Notification) {
	for _, item := range items {
		data, _ := json.Marshal(item)
		_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", item.Sequence, item.Kind, data)
	}
}

func (s *Server) subscribeElicitationWakeups(service *elicitation.Service) {
	service.Subscribe(func(event elicitation.Event) {
		if event.Kind != elicitation.EventOpened {
			return
		}
		req := event.Request
		runID := s.runStore.FindRunningRunForSession(req.AgentID, req.SessionID)
		_, _ = s.runStore.AppendNotification(runtrace.Notification{
			Kind: "elicitation.opened", RunID: runID, AgentID: req.AgentID,
			SessionID: req.SessionID, ElicitationID: req.ID,
		})
	})
}
