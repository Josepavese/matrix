package runaction

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

type deliveryStopReasonProbe struct {
	reasons []string
}

func (*deliveryStopReasonProbe) OnThought(middleware.ThoughtUpdate) {}
func (*deliveryStopReasonProbe) SetHeader(string, string)           {}
func (*deliveryStopReasonProbe) FormattedHeader() string            { return "" }

func (p *deliveryStopReasonProbe) OnTurnStopReason(stopReason string) {
	p.reasons = append(p.reasons, stopReason)
}

// The probe is declared as the canonical capability so that a change to that
// contract stops compiling here, next to the decorator that forwards it.
var _ runtrace.TurnStopReasonReporter = (*deliveryStopReasonProbe)(nil)

// TestDeliveryProofNotifierKeepsTurnStopReasonReachable pins the same decorator
// rule as the activity watchdog: wrapping a notifier to add delivery proof must
// not remove the optional capability that carries the provider's own stop reason
// to the run record.
func TestDeliveryProofNotifierKeepsTurnStopReasonReachable(t *testing.T) {
	probe := &deliveryStopReasonProbe{}
	wrapped := newAttachProofNotifier(probe, "delivery-stop-reason")

	reporter, ok := interface{}(wrapped).(runtrace.TurnStopReasonReporter)
	if !ok {
		t.Fatal("delivery proof wrapper dropped the TurnStopReasonReporter capability")
	}
	reporter.OnTurnStopReason("refusal")
	if len(probe.reasons) != 1 || probe.reasons[0] != "refusal" {
		t.Fatalf("stop reason did not reach the wrapped notifier unaltered: %#v", probe.reasons)
	}
}

// TestDeliveryProofNotifierAcceptsNotifiersWithoutTurnStopReason keeps the
// forward a no-op for notifiers that publish no stop reason.
func TestDeliveryProofNotifierAcceptsNotifiersWithoutTurnStopReason(t *testing.T) {
	wrapped := newAttachProofNotifier(&plainDeliveryNotifier{}, "delivery-plain")
	if _, ok := interface{}(wrapped).(runtrace.TurnStopReasonReporter); !ok {
		t.Fatal("the wrapper must always expose the optional capability")
	}
	wrapped.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeThinking, Content: "work"})
	wrapped.OnTurnStopReason("end_turn")
}
