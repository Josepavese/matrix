package runapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

func (s *Server) HandleEventSinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var req eventSinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return
	}
	sink, err := s.runStore.RegisterSink(runtrace.Sink{URL: req.URL, EventKinds: req.EventKinds, Metadata: req.Metadata})
	if err != nil {
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, sink)
}

func (s *Server) dispatchRunEvent(event runtrace.Event) {
	if s.sinkDelivery != nil {
		s.sinkDelivery.Dispatch(event)
	}
}

func (s *Server) StartSinkDeliveryWorker(ctx context.Context) {
	if s.sinkDelivery != nil {
		s.sinkDelivery.Start(ctx)
	}
}
