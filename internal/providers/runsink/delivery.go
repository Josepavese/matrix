package runsink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Josepavese/matrix/internal/logic/rundelivery"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

func (s *Service) Attempt(delivery rundelivery.Delivery) {
	sink, event, err := s.resolve(delivery)
	if err == nil {
		err = Post(sink, event)
	}
	if err != nil {
		if markErr := s.deliveries.MarkFailed(delivery.ID, err, maxAttempts); markErr != nil {
			slog.Warn("failed to mark run event delivery failed", "error", markErr, "delivery_id", delivery.ID)
		}
		return
	}
	if err := s.deliveries.MarkSent(delivery.ID); err != nil {
		slog.Warn("failed to mark run event delivery sent", "error", err, "delivery_id", delivery.ID)
	}
}

func (s *Service) resolve(delivery rundelivery.Delivery) (runtrace.Sink, runtrace.Event, error) {
	sink, found, err := s.runs.LoadSink(delivery.SinkID)
	if err != nil || !found {
		return runtrace.Sink{}, runtrace.Event{}, fmt.Errorf("sink %s not found", delivery.SinkID)
	}
	event, found, err := s.runs.LoadEvent(delivery.RunID, delivery.EventID)
	if err != nil || !found {
		return runtrace.Sink{}, runtrace.Event{}, fmt.Errorf("event %s not found", delivery.EventID)
	}
	return sink, event, nil
}

func Post(sink runtrace.Sink, event runtrace.Event) error {
	payload, err := json.Marshal(map[string]interface{}{"sink_id": sink.ID, "event": event})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, sink.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sink returned status %d", resp.StatusCode)
	}
	return nil
}

func sinkAccepts(sink runtrace.Sink, kind string) bool {
	if len(sink.EventKinds) == 0 {
		return true
	}
	for _, accepted := range sink.EventKinds {
		if accepted == kind || accepted == "*" {
			return true
		}
	}
	return false
}
