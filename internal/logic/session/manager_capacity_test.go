package session

import (
	"context"
	"errors"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/admission"
	"github.com/Josepavese/matrix/internal/middleware"
)

type capacityRouter struct{ entered chan struct{} }

func (r capacityRouter) Route(ctx context.Context, _ middleware.RouteRequest) (string, string, []middleware.ToolCall, middleware.ConversationMetadata, error) {
	close(r.entered)
	<-ctx.Done()
	return "", "", nil, middleware.ConversationMetadata{}, ctx.Err()
}

func TestCancelledProviderRouteReleasesAdmissionLease(t *testing.T) {
	entered := make(chan struct{})
	m := &Manager{router: capacityRouter{entered: entered}}
	gate := admission.New(nil, func() (admission.Limits, error) { return admission.Limits{MaxConcurrent: 1}, nil })
	m.WithAdmission(gate)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, _, _, err := m.routeWithCapacity(ctx, middleware.CapacityRequest{}, middleware.RouteRequest{})
		done <- err
	}()
	<-entered
	if _, err := gate.Acquire("", middleware.CapacityRequest{}); err == nil {
		t.Fatal("concurrent route admitted")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if gate.State().Active != 0 {
		t.Fatal("canceled route leaked its lease")
	}
}
