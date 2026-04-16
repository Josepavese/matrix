package runsink

import (
	"context"
	"log/slog"
	"time"

	"github.com/jose/matrix-v2/internal/logic/rundelivery"
	"github.com/jose/matrix-v2/internal/logic/runtrace"
)

const (
	maxAttempts = 8
	batchLimit  = 50
)

type Service struct {
	runs       *runtrace.Store
	deliveries *rundelivery.Store
}

func NewService(runs *runtrace.Store, deliveries *rundelivery.Store) *Service {
	return &Service{runs: runs, deliveries: deliveries}
}

func (s *Service) Dispatch(event runtrace.Event) {
	sinks, err := s.runs.ListSinks()
	if err != nil {
		slog.Warn("failed to list run event sinks", "error", err)
		return
	}
	for _, sink := range sinks {
		if sinkAccepts(sink, event.Kind) {
			s.enqueueAndAttempt(sink, event)
		}
	}
}

func (s *Service) Start(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processDue()
		}
	}
}

func (s *Service) processDue() {
	deliveries, err := s.deliveries.ListDue(time.Now().UTC(), batchLimit)
	if err != nil {
		slog.Warn("failed to load due run event deliveries", "error", err)
		return
	}
	for _, delivery := range deliveries {
		go s.Attempt(delivery)
	}
}

func (s *Service) enqueueAndAttempt(sink runtrace.Sink, event runtrace.Event) {
	delivery, err := s.deliveries.Enqueue(sink, event)
	if err != nil {
		slog.Warn("failed to enqueue run event sink delivery", "error", err, "sink_id", sink.ID)
		return
	}
	go s.Attempt(delivery)
}
